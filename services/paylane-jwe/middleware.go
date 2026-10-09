package jwe

import (
	"bytes"
	"context"
	"crypto/rsa"
	"io"
	"net/http"
	"strings"
)

type contextKey string

const (
	claimsContextKey  contextKey = "jwe_claims"
	payloadContextKey contextKey = "jwe_payload"
)

// GetClaims retrieves verified StandardClaims from context.
func GetClaims(ctx context.Context) *StandardClaims {
	if val, ok := ctx.Value(claimsContextKey).(*StandardClaims); ok {
		return val
	}
	return nil
}

// GetDecryptedBody retrieves decrypted raw bytes from context.
func GetDecryptedBody(ctx context.Context) []byte {
	if val, ok := ctx.Value(payloadContextKey).([]byte); ok {
		return val
	}
	return nil
}

// Middleware creates an HTTP handler that enforces JWE encryption and JWS signing.
func Middleware(
	serviceName string,
	receiverEncKey *rsa.PrivateKey,
	registry *KeyRegistry,
	replay ReplayProtector,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token: try body first, then fallback to Authorization header
			var token string
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil && len(bodyBytes) > 0 {
				token = strings.TrimSpace(string(bodyBytes))
			} else if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}

			if token == "" {
				http.Error(w, `{"error":"unauthorized: missing jwe token"}`, http.StatusUnauthorized)
				return
			}

			resolver := func(iss string) (*rsa.PublicKey, error) {
				return registry.GetSignPublicKey(iss)
			}

			payload, claims, err := Open(token, receiverEncKey, resolver, serviceName, replay)
			if err != nil {
				http.Error(w, `{"error":"unauthorized: `+err.Error()+`"}`, http.StatusUnauthorized)
				return
			}

			// Inject into request context and replace r.Body with decrypted payload
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			ctx = context.WithValue(ctx, payloadContextKey, payload)
			r = r.WithContext(ctx)
			r.Body = io.NopCloser(bytes.NewReader(payload))

			next.ServeHTTP(w, r)
		})
	}
}
