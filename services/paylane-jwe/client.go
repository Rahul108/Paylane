package jwe

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client handles signing and encrypting outbound HTTP requests and decrypting inbound responses.
type Client struct {
	serviceName    string
	senderSignKey  *rsa.PrivateKey
	receiverEncKey *rsa.PrivateKey
	registry       *KeyRegistry
	replay         ReplayProtector
	httpClient     *http.Client
}

// NewClient creates a new JWE-secured HTTP client.
func NewClient(
	serviceName string,
	senderSignKey *rsa.PrivateKey,
	receiverEncKey *rsa.PrivateKey,
	registry *KeyRegistry,
	replay ReplayProtector,
	httpClient *http.Client,
) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if replay == nil {
		replay = NewMemoryReplayProtector()
	}
	return &Client{
		serviceName:    serviceName,
		senderSignKey:  senderSignKey,
		receiverEncKey: receiverEncKey,
		registry:       registry,
		replay:         replay,
		httpClient:     httpClient,
	}
}

// Post sends a JWE-encrypted and signed POST request to the destination service.
func (c *Client) Post(ctx context.Context, destService, url string, payload []byte, headers map[string]string) (int, []byte, error) {
	return c.Do(ctx, http.MethodPost, destService, url, payload, headers)
}

// Do executes a signed and encrypted HTTP call.
func (c *Client) Do(
	ctx context.Context,
	method, destService, url string,
	payload []byte,
	headers map[string]string,
) (int, []byte, error) {
	destEncKey, err := c.registry.GetEncPublicKey(destService)
	if err != nil {
		return 0, nil, fmt.Errorf("lookup enc public key for %s: %w", destService, err)
	}

	token, err := Seal(payload, c.senderSignKey, destEncKey, c.serviceName, destService, 60*time.Second)
	if err != nil {
		return 0, nil, fmt.Errorf("seal outbound payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(token))
	if err != nil {
		return 0, nil, fmt.Errorf("create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/jose")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("execute http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return resp.StatusCode, respBytes, nil
	}

	// If response is encrypted JWE token
	respStr := strings.TrimSpace(string(respBytes))
	if len(respStr) > 0 && strings.Count(respStr, ".") == 4 { // JWE has 5 parts separated by 4 dots
		resolver := func(iss string) (*rsa.PublicKey, error) {
			return c.registry.GetSignPublicKey(iss)
		}
		decrypted, _, err := Open(respStr, c.receiverEncKey, resolver, c.serviceName, c.replay)
		if err != nil {
			return resp.StatusCode, nil, fmt.Errorf("failed to open response token: %w", err)
		}
		return resp.StatusCode, decrypted, nil
	}

	return resp.StatusCode, respBytes, nil
}
