package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"paylane-jwe"
)

func main() {
	keysDir := os.Getenv("KEYS_DIR")
	if keysDir == "" {
		keysDir = "../../keys"
	}
	signKeyPath := filepath.Join(keysDir, "web", "sign_private.pem")
	signKey, err := jwe.LoadRSAPrivateKey(signKeyPath)
	if err != nil {
		fmt.Printf("FAIL: load web sign key: %v\n", err)
		os.Exit(1)
	}

	encKeyPath := filepath.Join(keysDir, "web", "enc_private.pem")
	encKey, _ := jwe.LoadRSAPrivateKey(encKeyPath)

	registry := jwe.NewKeyRegistry(keysDir)
	replay := jwe.NewMemoryReplayProtector()
	client := jwe.NewClient("web", signKey, encKey, registry, replay, nil)

	ctx := context.Background()

	fmt.Println("==> 1. Initiating Payment on payment-core over JWE...")
	initReq := map[string]any{
		"idempotency_key": fmt.Sprintf("e2e_%d", time.Now().UnixNano()),
		"customer_id":     "cust_e2e_user",
		"amount":          50000, // 500.00 BDT
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"return_url":      "http://localhost:3010/return",
	}
	initBytes, _ := json.Marshal(initReq)

	status, respBytes, err := client.Post(ctx, "payment-core", "http://localhost:4011/payments", initReqBytes(initBytes), nil)
	if err != nil || status != 201 {
		fmt.Printf("FAIL: Initiate payment failed (status %d): %v, resp: %s\n", status, err, string(respBytes))
		os.Exit(1)
	}

	var initResp struct {
		Payment struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"payment"`
		RedirectURL string `json:"redirect_url"`
		SessionID   string `json:"session_id"`
	}
	_ = json.Unmarshal(respBytes, &initResp)
	paymentID := initResp.Payment.ID
	sessionID := initResp.SessionID
	fmt.Printf("  ✅ Payment Created: ID=%s Status=%s SessionID=%s\n", paymentID, initResp.Payment.Status, sessionID)

	fmt.Println("==> 2. Submitting PIN/OTP on mock-mfs hosted checkout...")
	form := url.Values{}
	form.Set("session_id", sessionID)
	form.Set("msisdn", "01700000000")
	form.Set("pin", "1234")
	form.Set("otp", "123456")
	form.Set("scenario", "SUCCESS")

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, _ := http.NewRequest(http.MethodPost, "http://localhost:5011/checkout/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		fmt.Printf("FAIL: submit checkout: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	fmt.Printf("  ✅ Checkout Form Submitted: HTTP Status=%d, Location=%s\n", resp.StatusCode, resp.Header.Get("Location"))

	// Wait for async webhook callback
	time.Sleep(1 * time.Second)

	fmt.Println("==> 3. Verifying Payment Status on payment-core...")
	status, pBytes, err := client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", paymentID), nil, nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: get payment failed: %v (%s)\n", err, string(pBytes))
		os.Exit(1)
	}

	var pData struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(pBytes, &pData)
	fmt.Printf("  ✅ Payment Status: %s (Expected: CAPTURED)\n", pData.Status)
	if pData.Status != "CAPTURED" {
		fmt.Printf("FAIL: Expected CAPTURED, got %s\n", pData.Status)
		os.Exit(1)
	}

	fmt.Println("==> 4. Verifying Double-Entry Ledger entries...")
	status, ledgerBytes, err := client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s/ledger", paymentID), nil, nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: get ledger failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Ledger entries: %s\n", string(ledgerBytes))

	fmt.Println("\n==> 5. Testing Card Gateway Flow (mock-card)...")
	cardInitReq := map[string]any{
		"idempotency_key": fmt.Sprintf("e2e_card_%d", time.Now().UnixNano()),
		"customer_id":     "cust_card_user",
		"amount":          75000, // 750.00 BDT
		"currency":        "BDT",
		"method":          "card",
		"provider":        "mock-card",
		"return_url":      "http://localhost:3010/return",
	}
	cardInitBytes, _ := json.Marshal(cardInitReq)
	status, respBytes, err = client.Post(ctx, "payment-core", "http://localhost:4011/payments", cardInitBytes, nil)
	if err != nil || status != 201 {
		fmt.Printf("FAIL: Card initiate failed: %v (%s)\n", err, string(respBytes))
		os.Exit(1)
	}

	var cardInitResp struct {
		Payment struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"payment"`
		RedirectURL string `json:"redirect_url"`
		SessionID   string `json:"session_id"`
	}
	_ = json.Unmarshal(respBytes, &cardInitResp)
	cardPaymentID := cardInitResp.Payment.ID
	cardSessionID := cardInitResp.SessionID
	fmt.Printf("  ✅ Card Payment Created: ID=%s SessionID=%s\n", cardPaymentID, cardSessionID)

	cardForm := url.Values{}
	cardForm.Set("session_id", cardSessionID)
	cardForm.Set("cardholder", "Jane Doe")
	cardForm.Set("pan", "4111111111111111")
	cardForm.Set("expiry", "12/28")
	cardForm.Set("cvv", "123")
	cardForm.Set("otp", "123456")
	cardForm.Set("scenario", "SUCCESS")

	cardReq, _ := http.NewRequest(http.MethodPost, "http://localhost:5012/checkout/confirm", strings.NewReader(cardForm.Encode()))
	cardReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cardResp, err := noRedirectClient.Do(cardReq)
	if err != nil {
		fmt.Printf("FAIL: submit card checkout: %v\n", err)
		os.Exit(1)
	}
	defer cardResp.Body.Close()
	fmt.Printf("  ✅ Card Checkout Submitted: HTTP Status=%d, Location=%s\n", cardResp.StatusCode, cardResp.Header.Get("Location"))

	time.Sleep(1 * time.Second)

	status, pBytes, err = client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", cardPaymentID), nil, nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: get card payment: %v (%s)\n", err, string(pBytes))
		os.Exit(1)
	}
	_ = json.Unmarshal(pBytes, &pData)
	fmt.Printf("  ✅ Card Payment Status: %s (Expected: CAPTURED)\n", pData.Status)
	if pData.Status != "CAPTURED" {
		fmt.Printf("FAIL: Expected CAPTURED, got %s\n", pData.Status)
		os.Exit(1)
	}

	fmt.Println("==========================================================")
	fmt.Println("   🎉 ALL MOCK PGWS END-TO-END PAYMENTS SUCCEEDED (PHASE 3)!")
	fmt.Println("==========================================================")
}

func initReqBytes(b []byte) []byte {
	return b
}
