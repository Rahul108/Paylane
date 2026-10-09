package jwe

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// LoadRSAPrivateKey loads an RSA private key from a PEM-encoded file.
func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}
	return ParseRSAPrivateKeyPEM(data)
}

// ParseRSAPrivateKeyPEM parses PEM bytes into an RSA private key (supports PKCS#1 and PKCS#8).
func ParseRSAPrivateKeyPEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to parse PEM block for private key")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	rawKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key (PKCS1/PKCS8): %w", err)
	}

	rsaKey, ok := rawKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an RSA private key")
	}
	return rsaKey, nil
}

// LoadRSAPublicKey loads an RSA public key from a PEM-encoded file.
func LoadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}
	return ParseRSAPublicKeyPEM(data)
}

// ParseRSAPublicKeyPEM parses PEM bytes into an RSA public key.
func ParseRSAPublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to parse PEM block for public key")
	}

	rawKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try PKCS#1 public key
		if key, pkcs1Err := x509.ParsePKCS1PublicKey(block.Bytes); pkcs1Err == nil {
			return key, nil
		}
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaKey, ok := rawKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("key is not an RSA public key")
	}
	return rsaKey, nil
}

// KeyRegistry manages public keys for all services loaded from a directory.
type KeyRegistry struct {
	baseDir string
	mu      sync.RWMutex
	signPub map[string]*rsa.PublicKey
	encPub  map[string]*rsa.PublicKey
}

// NewKeyRegistry loads public keys from the keys root directory.
// Layout expected: baseDir/<service>/sign_public.pem and baseDir/<service>/enc_public.pem
func NewKeyRegistry(baseDir string) *KeyRegistry {
	return &KeyRegistry{
		baseDir: baseDir,
		signPub: make(map[string]*rsa.PublicKey),
		encPub:  make(map[string]*rsa.PublicKey),
	}
}

// GetSignPublicKey returns the public key used to verify signatures from issuer.
func (r *KeyRegistry) GetSignPublicKey(serviceName string) (*rsa.PublicKey, error) {
	r.mu.RLock()
	key, exists := r.signPub[serviceName]
	r.mu.RUnlock()
	if exists {
		return key, nil
	}

	path := filepath.Join(r.baseDir, serviceName, "sign_public.pem")
	pub, err := LoadRSAPublicKey(path)
	if err != nil {
		return nil, fmt.Errorf("load sign public key for service %q from %q: %w", serviceName, path, err)
	}

	r.mu.Lock()
	r.signPub[serviceName] = pub
	r.mu.Unlock()
	return pub, nil
}

// GetEncPublicKey returns the public key used to encrypt payloads intended for receiver.
func (r *KeyRegistry) GetEncPublicKey(serviceName string) (*rsa.PublicKey, error) {
	r.mu.RLock()
	key, exists := r.encPub[serviceName]
	r.mu.RUnlock()
	if exists {
		return key, nil
	}

	path := filepath.Join(r.baseDir, serviceName, "enc_public.pem")
	pub, err := LoadRSAPublicKey(path)
	if err != nil {
		return nil, fmt.Errorf("load encryption public key for service %q from %q: %w", serviceName, path, err)
	}

	r.mu.Lock()
	r.encPub[serviceName] = pub
	r.mu.Unlock()
	return pub, nil
}
