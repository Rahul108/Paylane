package jwe

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// Seal signs the payload with the sender's private key (JWS RS256)
// and then encrypts the JWS with the receiver's public key (JWE RSA-OAEP-256 + A256GCM).
func Seal(
	payload []byte,
	senderSignKey *rsa.PrivateKey,
	receiverEncPublicKey *rsa.PublicKey,
	iss, aud string,
	ttl time.Duration,
) (string, error) {
	if senderSignKey == nil {
		return "", fmt.Errorf("sender signing private key is nil")
	}
	if receiverEncPublicKey == nil {
		return "", fmt.Errorf("receiver encryption public key is nil")
	}

	claims := NewStandardClaims(iss, aud, ttl)
	nested := NestedPayload{
		Claims: claims,
		Data:   payload,
	}

	nestedJSON, err := json.Marshal(nested)
	if err != nil {
		return "", fmt.Errorf("marshal nested payload: %w", err)
	}

	// 1. Sign (JWS RS256)
	signerKey := jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       senderSignKey,
	}
	signerOpts := (&jose.SignerOptions{}).WithType("JWT")
	signer, err := jose.NewSigner(signerKey, signerOpts)
	if err != nil {
		return "", fmt.Errorf("create jws signer: %w", err)
	}

	signedObj, err := signer.Sign(nestedJSON)
	if err != nil {
		return "", fmt.Errorf("sign payload: %w", err)
	}

	jwsCompact, err := signedObj.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("serialize jws: %w", err)
	}

	// 2. Encrypt (JWE RSA-OAEP-256 / A256GCM)
	recipient := jose.Recipient{
		Algorithm: jose.RSA_OAEP_256,
		Key:       receiverEncPublicKey,
	}
	encOpts := (&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT")
	encrypter, err := jose.NewEncrypter(jose.A256GCM, recipient, encOpts)
	if err != nil {
		return "", fmt.Errorf("create jwe encrypter: %w", err)
	}

	encryptedObj, err := encrypter.Encrypt([]byte(jwsCompact))
	if err != nil {
		return "", fmt.Errorf("encrypt jws payload: %w", err)
	}

	jweCompact, err := encryptedObj.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("serialize jwe: %w", err)
	}

	return jweCompact, nil
}
