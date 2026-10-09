package jwe

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// SenderKeyResolver returns the public signing key for the given issuer.
type SenderKeyResolver func(iss string) (*rsa.PublicKey, error)

// Open decrypts the incoming JWE token, verifies the inner JWS signature,
// validates audience, expiry, and ensures the JTI has not been replayed.
func Open(
	tokenString string,
	receiverEncKey *rsa.PrivateKey,
	resolveSender SenderKeyResolver,
	expectedAud string,
	replay ReplayProtector,
) ([]byte, *StandardClaims, error) {
	if receiverEncKey == nil {
		return nil, nil, fmt.Errorf("receiver decryption private key is nil")
	}

	// 1. Decrypt JWE
	jweObj, err := jose.ParseEncrypted(tokenString, []jose.KeyAlgorithm{jose.RSA_OAEP_256}, []jose.ContentEncryption{jose.A256GCM})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse encrypted token: %v", ErrDecryptionFailed, err)
	}

	decryptedJWS, err := jweObj.Decrypt(receiverEncKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decrypt token: %v", ErrDecryptionFailed, err)
	}

	// 2. Parse JWS
	jwsObj, err := jose.ParseSigned(string(decryptedJWS), []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse signed token: %v", ErrSignatureInvalid, err)
	}

	// Unsafely inspect the payload to get issuer for key resolution
	var peek NestedPayload
	unverified := jwsObj.UnsafePayloadWithoutVerification()
	if err := json.Unmarshal(unverified, &peek); err != nil {
		return nil, nil, fmt.Errorf("%w: inspect payload: %v", ErrMissingClaims, err)
	}

	if peek.Claims.Issuer == "" {
		return nil, nil, fmt.Errorf("%w: empty issuer", ErrInvalidIssuer)
	}

	// Resolve sender's public signing key
	senderSignPub, err := resolveSender(peek.Claims.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: resolve issuer %q key: %v", ErrInvalidIssuer, peek.Claims.Issuer, err)
	}

	// 3. Verify signature
	verifiedBytes, err := jwsObj.Verify(senderSignPub)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: verify signature: %v", ErrSignatureInvalid, err)
	}

	var verified NestedPayload
	if err := json.Unmarshal(verifiedBytes, &verified); err != nil {
		return nil, nil, fmt.Errorf("unmarshal verified payload: %w", err)
	}

	claims := &verified.Claims

	// 4. Validate Claims
	if expectedAud != "" && claims.Audience != expectedAud {
		return nil, nil, fmt.Errorf("%w: expected %q, got %q", ErrInvalidAudience, expectedAud, claims.Audience)
	}

	now := time.Now().UTC()
	// Allow 5 seconds clock skew
	if claims.ExpiresAt <= now.Add(-5*time.Second).Unix() {
		return nil, nil, fmt.Errorf("%w: expired at %v", ErrTokenExpired, time.Unix(claims.ExpiresAt, 0).UTC())
	}
	if claims.IssuedAt > now.Add(5*time.Second).Unix() {
		return nil, nil, fmt.Errorf("%w: issued in future at %v", ErrTokenNotYetValid, time.Unix(claims.IssuedAt, 0).UTC())
	}

	// 5. Replay Protection
	if replay != nil && claims.ID != "" {
		expTime := time.Unix(claims.ExpiresAt, 0).UTC()
		if !replay.CheckAndRecord(claims.ID, expTime) {
			return nil, nil, fmt.Errorf("%w: jti=%s", ErrTokenReplayed, claims.ID)
		}
	}

	return verified.Data, claims, nil
}
