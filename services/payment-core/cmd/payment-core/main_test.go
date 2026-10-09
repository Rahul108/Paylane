package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"paylane-jwe"
	"payment-core/internal/handler"
)

func writeKeyPEM(t *testing.T, path string, priv *rsa.PrivateKey) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(path, privPEM, 0600); err != nil {
		t.Fatalf("failed to write private key: %v", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	pubPath := strings.Replace(path, "private.pem", "public.pem", 1)
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		t.Fatalf("failed to write public key: %v", err)
	}
}

func TestPaymentCore_Endpoints(t *testing.T) {
	tempKeysDir := t.TempDir()

	svcSignKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	svcEncKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	callerSignKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	callerEncKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	writeKeyPEM(t, filepath.Join(tempKeysDir, "payment-core", "sign_private.pem"), svcSignKey)
	writeKeyPEM(t, filepath.Join(tempKeysDir, "payment-core", "enc_private.pem"), svcEncKey)
	writeKeyPEM(t, filepath.Join(tempKeysDir, "caller", "sign_private.pem"), callerSignKey)
	writeKeyPEM(t, filepath.Join(tempKeysDir, "caller", "enc_private.pem"), callerEncKey)

	registry := jwe.NewKeyRegistry(tempKeysDir)
	replay := jwe.NewMemoryReplayProtector()
	healthHandler := handler.NewHealthHandler("payment-core", svcSignKey, registry)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", healthHandler.Livez)

	jweAuth := jwe.Middleware("payment-core", svcEncKey, registry, replay)
	mux.Handle("POST /health", jweAuth(http.HandlerFunc(healthHandler.Health)))

	// 1. Livez without auth: 200 OK
	reqLivez := httptest.NewRequest(http.MethodGet, "/livez", nil)
	recLivez := httptest.NewRecorder()
	mux.ServeHTTP(recLivez, reqLivez)
	if recLivez.Code != http.StatusOK {
		t.Fatalf("expected 200 for livez, got %d", recLivez.Code)
	}

	// 2. Health without token: 401 Unauthorized
	reqUnauth := httptest.NewRequest(http.MethodPost, "/health", nil)
	recUnauth := httptest.NewRecorder()
	mux.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated health, got %d", recUnauth.Code)
	}

	// 3. Health with valid signed & encrypted JWE token: 200 OK
	token, err := jwe.Seal([]byte(`{"ping":true}`), callerSignKey, &svcEncKey.PublicKey, "caller", "payment-core", 60*time.Second)
	if err != nil {
		t.Fatalf("failed to seal token: %v", err)
	}

	reqAuth := httptest.NewRequest(http.MethodPost, "/health", strings.NewReader(token))
	recAuth := httptest.NewRecorder()
	mux.ServeHTTP(recAuth, reqAuth)
	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated health, got %d. Body: %s", recAuth.Code, recAuth.Body.String())
	}

	// Verify response from health is also JWE encrypted
	resolver := func(iss string) (*rsa.PublicKey, error) {
		return registry.GetSignPublicKey(iss)
	}
	respPayload, respClaims, err := jwe.Open(recAuth.Body.String(), callerEncKey, resolver, "caller", nil)
	if err != nil {
		t.Fatalf("failed to decrypt health response: %v", err)
	}
	if respClaims.Issuer != "payment-core" || respClaims.Audience != "caller" {
		t.Fatalf("unexpected response claims: %+v", respClaims)
	}
	if !strings.Contains(string(respPayload), `"status":"healthy"`) {
		t.Fatalf("unexpected decrypted body: %s", string(respPayload))
	}
}
