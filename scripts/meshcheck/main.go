package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"paylane-jwe"
)

type Target struct {
	Name string
	URL  string
}

func main() {
	keysDir := os.Getenv("KEYS_DIR")
	if keysDir == "" {
		keysDir = "./keys"
	}

	targets := []Target{
		{Name: "orchestrator", URL: "http://localhost:4010/health"},
		{Name: "payment-core", URL: "http://localhost:4011/health"},
		{Name: "mock-mfs", URL: "http://localhost:5011/health"},
		{Name: "mock-card", URL: "http://localhost:5012/health"},
		{Name: "mock-downstream", URL: "http://localhost:5013/health"},
	}

	signKeyPath := filepath.Join(keysDir, "web", "sign_private.pem")
	signKey, err := jwe.LoadRSAPrivateKey(signKeyPath)
	if err != nil {
		fmt.Printf("FAIL: load web sign key: %v\n", err)
		os.Exit(1)
	}

	encKeyPath := filepath.Join(keysDir, "web", "enc_private.pem")
	encKey, err := jwe.LoadRSAPrivateKey(encKeyPath)
	if err != nil {
		fmt.Printf("FAIL: load web enc key: %v\n", err)
	}

	registry := jwe.NewKeyRegistry(keysDir)
	replay := jwe.NewMemoryReplayProtector()
	client := jwe.NewClient("web", signKey, encKey, registry, replay, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	allPassed := true
	for _, target := range targets {
		status, resp, err := client.Post(ctx, target.Name, target.URL, []byte(`{"ping":true}`), nil)
		if err != nil || status != 200 {
			fmt.Printf("❌ [%s] failed: status=%d err=%v body=%s\n", target.Name, status, err, string(resp))
			allPassed = false
		} else {
			fmt.Printf("✅ [%s] verified JWE mutual handshake! Status=%d Response=%s\n", target.Name, status, string(resp))
		}
	}

	if !allPassed {
		os.Exit(1)
	}
	fmt.Println("==> All JWE service mesh handshakes passed successfully!")
}
