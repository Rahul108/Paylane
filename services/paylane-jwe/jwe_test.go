package jwe

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func generateTestKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate sign key: %v", err)
	}
	encKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate enc key: %v", err)
	}
	return signKey, encKey
}

func TestJWE_SealAndOpen_Success(t *testing.T) {
	senderSignKey, _ := generateTestKeys(t)
	_, receiverEncKey := generateTestKeys(t)

	payload := []byte(`{"message":"hello paylane","amount":1000}`)
	token, err := Seal(payload, senderSignKey, &receiverEncKey.PublicKey, "sender-service", "receiver-service", 60*time.Second)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	resolver := func(iss string) (*rsa.PublicKey, error) {
		if iss != "sender-service" {
			t.Fatalf("unexpected issuer: %s", iss)
		}
		return &senderSignKey.PublicKey, nil
	}

	replay := NewMemoryReplayProtector()
	decrypted, claims, err := Open(token, receiverEncKey, resolver, "receiver-service", replay)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if string(decrypted) != string(payload) {
		t.Errorf("decrypted payload mismatch. Expected %s, got %s", payload, decrypted)
	}

	if claims.Issuer != "sender-service" || claims.Audience != "receiver-service" {
		t.Errorf("claims mismatch: %+v", claims)
	}
}

func TestJWE_TamperedToken_Rejected(t *testing.T) {
	senderSignKey, _ := generateTestKeys(t)
	_, receiverEncKey := generateTestKeys(t)

	payload := []byte(`{"test":"tamper"}`)
	token, err := Seal(payload, senderSignKey, &receiverEncKey.PublicKey, "sender", "receiver", 60*time.Second)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	// Tamper with the token characters
	tampered := token[:len(token)-5] + "XXXXX"

	resolver := func(iss string) (*rsa.PublicKey, error) {
		return &senderSignKey.PublicKey, nil
	}

	_, _, err = Open(tampered, receiverEncKey, resolver, "receiver", nil)
	if err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
}

func TestJWE_WrongAudience_Rejected(t *testing.T) {
	senderSignKey, _ := generateTestKeys(t)
	_, receiverEncKey := generateTestKeys(t)

	token, err := Seal([]byte("foo"), senderSignKey, &receiverEncKey.PublicKey, "sender", "receiver-a", 60*time.Second)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	resolver := func(iss string) (*rsa.PublicKey, error) {
		return &senderSignKey.PublicKey, nil
	}

	_, _, err = Open(token, receiverEncKey, resolver, "receiver-b", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid audience") {
		t.Fatalf("expected ErrInvalidAudience, got: %v", err)
	}
}

func TestJWE_ExpiredToken_Rejected(t *testing.T) {
	senderSignKey, _ := generateTestKeys(t)
	_, receiverEncKey := generateTestKeys(t)

	// Seal with negative TTL
	token, err := Seal([]byte("foo"), senderSignKey, &receiverEncKey.PublicKey, "sender", "receiver", -10*time.Second)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	resolver := func(iss string) (*rsa.PublicKey, error) {
		return &senderSignKey.PublicKey, nil
	}

	_, _, err = Open(token, receiverEncKey, resolver, "receiver", nil)
	if err == nil || !strings.Contains(err.Error(), "token is expired") {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestJWE_ReplayToken_Rejected(t *testing.T) {
	senderSignKey, _ := generateTestKeys(t)
	_, receiverEncKey := generateTestKeys(t)

	token, err := Seal([]byte("foo"), senderSignKey, &receiverEncKey.PublicKey, "sender", "receiver", 60*time.Second)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	resolver := func(iss string) (*rsa.PublicKey, error) {
		return &senderSignKey.PublicKey, nil
	}

	replay := NewMemoryReplayProtector()
	// First open: should succeed
	_, _, err = Open(token, receiverEncKey, resolver, "receiver", replay)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}

	// Second open with same token: must fail with replay error
	_, _, err = Open(token, receiverEncKey, resolver, "receiver", replay)
	if err == nil || !strings.Contains(err.Error(), "token jti replayed") {
		t.Fatalf("expected ErrTokenReplayed, got: %v", err)
	}
}

func TestJWE_Middleware_MissingToken(t *testing.T) {
	_, receiverEncKey := generateTestKeys(t)
	middleware := Middleware("receiver", receiverEncKey, NewKeyRegistry(""), nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %d", rec.Code)
	}
}
