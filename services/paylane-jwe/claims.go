package jwe

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	ErrTokenExpired       = errors.New("jwe: token is expired")
	ErrTokenNotYetValid   = errors.New("jwe: token not yet valid")
	ErrInvalidAudience    = errors.New("jwe: invalid audience")
	ErrInvalidIssuer      = errors.New("jwe: invalid issuer")
	ErrTokenReplayed      = errors.New("jwe: token jti replayed")
	ErrSignatureInvalid   = errors.New("jwe: signature verification failed")
	ErrDecryptionFailed   = errors.New("jwe: decryption failed")
	ErrMissingClaims      = errors.New("jwe: required claims missing")
)

// StandardClaims encapsulates the standard JWT claims for service-to-service JWE.
type StandardClaims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	ID        string `json:"jti"`
}

// NewStandardClaims creates standard claims with a generated ULID and 60-second default TTL.
func NewStandardClaims(iss, aud string, ttl time.Duration) StandardClaims {
	now := time.Now().UTC()
	if ttl == 0 {
		ttl = 60 * time.Second
	}
	t := now
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(t), entropy).String()

	return StandardClaims{
		Issuer:    iss,
		Audience:  aud,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		ID:        id,
	}
}

// NestedPayload encapsulates user payload alongside standard claims before signing.
type NestedPayload struct {
	Claims StandardClaims `json:"claims"`
	Data   []byte         `json:"data"`
}
