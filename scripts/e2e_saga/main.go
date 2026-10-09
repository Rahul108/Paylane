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

type Binding struct {
	ID               string `json:"id"`
	CustomerID       string `json:"customer_id"`
	Method           string `json:"method"`
	Provider         string `json:"provider"`
	TokenReference   string `json:"token_reference"`
	MaskedIdentifier string `json:"masked_identifier"`
	Status           string `json:"status"`
}

type JourneyStep struct {
	ID                  string  `json:"id"`
	StepName            string  `json:"step_name"`
	StepOrder           int     `json:"step_order"`
	Status              string  `json:"status"`
	AttemptCount        int     `json:"attempt_count"`
	LastError           *string `json:"last_error,omitempty"`
	DownstreamReference *string `json:"downstream_reference,omitempty"`
}

type Journey struct {
	ID          string        `json:"id"`
	JourneyType string        `json:"journey_type"`
	CustomerID  string        `json:"customer_id"`
	Status      string        `json:"status"`
	CurrentStep *string       `json:"current_step,omitempty"`
	PaymentID   *string       `json:"payment_id,omitempty"`
	Steps       []JourneyStep `json:"steps"`
}

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

	fmt.Println("==========================================================")
	fmt.Println("  PHASE 4: TOKEN BINDING, UNBIND & UI-LESS PAYMENT CHECKS ")
	fmt.Println("==========================================================")

	// 1. Confirm a new binding on payment-core
	fmt.Println("==> 4.1 Confirming wallet token binding on payment-core...")
	p4CustID := fmt.Sprintf("cust_p4_%d", time.Now().UnixNano())
	p4Token := fmt.Sprintf("tok_mfs_%d", time.Now().UnixNano())
	bindReq := map[string]any{
		"customer_id":       p4CustID,
		"method":            "mfs",
		"provider":          "mock-mfs",
		"token_reference":   p4Token,
		"masked_identifier": "017***12345",
	}
	bindBytes, _ := json.Marshal(bindReq)
	status, respBytes, err := client.Post(ctx, "payment-core", "http://localhost:4011/bindings/confirm", bindBytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Confirm binding failed (status %d): %v, resp: %s\n", status, err, string(respBytes))
		os.Exit(1)
	}
	var createdBinding Binding
	_ = json.Unmarshal(respBytes, &createdBinding)
	fmt.Printf("  ✅ Binding Created: ID=%s, Status=%s, TokenRef=%s\n", createdBinding.ID, createdBinding.Status, createdBinding.TokenReference)

	// 2. Query customer bindings
	fmt.Println("==> 4.2 Querying active bindings for customer...")
	queryURL := fmt.Sprintf("http://localhost:4011/customers/%s/bindings", p4CustID)
	status, listBytes, err := client.Do(ctx, http.MethodGet, "payment-core", queryURL, nil, nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Query bindings failed: %v (%s)\n", err, string(listBytes))
		os.Exit(1)
	}
	var listResp struct {
		CustomerID string    `json:"customer_id"`
		Bindings   []Binding `json:"bindings"`
	}
	_ = json.Unmarshal(listBytes, &listResp)
	bindings := listResp.Bindings
	if len(bindings) == 0 || bindings[0].TokenReference != p4Token {
		fmt.Printf("FAIL: Expected active binding in list, got: %s\n", string(listBytes))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Customer has %d active binding(s)\n", len(bindings))

	// 3. Test Unbound/Foreign Token Rejection
	fmt.Println("==> 4.3 Testing rejection of unbound/foreign token reference...")
	unboundPayReq := map[string]any{
		"idempotency_key": fmt.Sprintf("unbound_test_%d", time.Now().UnixNano()),
		"customer_id":     p4CustID,
		"amount":          10000,
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"token_reference": "tok_unbound_invalid_999",
	}
	unboundPayBytes, _ := json.Marshal(unboundPayReq)
	status, rejResp, _ := client.Post(ctx, "payment-core", "http://localhost:4011/payments", unboundPayBytes, nil)
	if status == 200 || status == 201 {
		fmt.Printf("FAIL: Expected rejection for unbound token, but got success: %d\n", status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Unbound token correctly rejected! (HTTP status %d: %s)\n", status, string(rejResp))

	// 4. Test UI-less Payment with Bound Token
	fmt.Println("==> 4.4 Executing UI-less Payment using bound token...")
	uilessPayReq := map[string]any{
		"idempotency_key": fmt.Sprintf("uiless_test_%d", time.Now().UnixNano()),
		"customer_id":     p4CustID,
		"amount":          25000, // 250.00 BDT
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"token_reference": p4Token,
	}
	uilessPayBytes, _ := json.Marshal(uilessPayReq)
	status, uilessResp, err := client.Post(ctx, "payment-core", "http://localhost:4011/payments", uilessPayBytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: UI-less payment failed: %v (%s)\n", err, string(uilessResp))
		os.Exit(1)
	}
	var uilessData struct {
		Payment struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"payment"`
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(uilessResp, &uilessData)
	fmt.Printf("  ✅ UI-less Payment: ID=%s, Status=%s, RedirectURL='%s'\n", uilessData.Payment.ID, uilessData.Payment.Status, uilessData.RedirectURL)
	if uilessData.Payment.Status != "CAPTURED" || uilessData.RedirectURL != "" {
		fmt.Printf("FAIL: Expected immediate CAPTURED without redirect for UI-less payment, got status=%s, redirect=%s\n", uilessData.Payment.Status, uilessData.RedirectURL)
		os.Exit(1)
	}

	// 5. Unbind Token
	fmt.Println("==> 4.5 Revoking (unbinding) token...")
	unbindURL := fmt.Sprintf("http://localhost:4011/bindings/%s/unbind", createdBinding.ID)
	status, unbindResp, err := client.Post(ctx, "payment-core", unbindURL, []byte("{}"), nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Unbind failed: %v (%s)\n", err, string(unbindResp))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Token unbind completed successfully\n")

	// 6. Test Payment with Revoked Token Rejection
	fmt.Println("==> 4.6 Verifying payment rejection after unbind...")
	revPayReq := map[string]any{
		"idempotency_key": fmt.Sprintf("revoked_test_%d", time.Now().UnixNano()),
		"customer_id":     p4CustID,
		"amount":          25000,
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"token_reference": p4Token,
	}
	revPayBytes, _ := json.Marshal(revPayReq)
	status, revResp, _ := client.Post(ctx, "payment-core", "http://localhost:4011/payments", revPayBytes, nil)
	if status == 200 || status == 201 {
		fmt.Printf("FAIL: Expected rejection for revoked token, but got success: %d\n", status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Revoked token payment correctly rejected! (HTTP %d: %s)\n", status, string(revResp))

	fmt.Println("\n==========================================================")
	fmt.Println("      PHASE 5: ORCHESTRATOR SAGA ENGINE VERIFICATION      ")
	fmt.Println("==========================================================")

	// Bind card token for UI-less saga test
	bindCardReq := map[string]any{
		"customer_id":       "cust_saga_user",
		"method":            "card",
		"provider":          "mock-card",
		"token_reference":   "tok_card_saga_9988",
		"masked_identifier": "4111****1111",
	}
	bindCardBytes, _ := json.Marshal(bindCardReq)
	_, _, _ = client.Post(ctx, "payment-core", "http://localhost:4011/bindings/confirm", bindCardBytes, nil)

	// 5.1 Single-step UI-less journey
	fmt.Println("==> 5.1 Starting 'uiless_payment' journey...")
	journey1Req := map[string]any{
		"journey_type":    "uiless_payment",
		"customer_id":     "cust_saga_user",
		"amount":          15000,
		"currency":        "BDT",
		"method":          "card",
		"provider":        "mock-card",
		"token_reference": "tok_card_saga_9988",
	}
	j1Bytes, _ := json.Marshal(journey1Req)
	status, j1RespBytes, err := client.Post(ctx, "orchestrator", "http://localhost:4010/journeys", j1Bytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Start uiless journey failed: %v (%s)\n", err, string(j1RespBytes))
		os.Exit(1)
	}
	var j1Resp struct {
		JourneyID string `json:"journey_id"`
		Status    string `json:"status"`
	}
	_ = json.Unmarshal(j1RespBytes, &j1Resp)
	fmt.Printf("  ✅ Journey Created: ID=%s, Status=%s\n", j1Resp.JourneyID, j1Resp.Status)
	if j1Resp.Status != "COMPLETED" {
		fmt.Printf("FAIL: Expected uiless_payment to complete synchronously, got %s\n", j1Resp.Status)
		os.Exit(1)
	}

	// 5.2 Multi-step saga (payment_recharge) with happy path
	fmt.Println("\n==> 5.2 Starting 'payment_recharge' journey (Happy Path)...")
	journey2Req := map[string]any{
		"journey_type":        "payment_recharge",
		"customer_id":         "cust_saga_user",
		"amount":              30000,
		"currency":            "BDT",
		"method":              "mfs",
		"provider":            "mock-mfs",
		"mobile_number":       "01712345678",
		"downstream_scenario": "SUCCESS",
		"return_url":          "http://localhost:3010/return",
	}
	j2Bytes, _ := json.Marshal(journey2Req)
	status, j2RespBytes, err := client.Post(ctx, "orchestrator", "http://localhost:4010/journeys", j2Bytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Start recharge journey failed: %v (%s)\n", err, string(j2RespBytes))
		os.Exit(1)
	}
	var j2Resp struct {
		JourneyID   string `json:"journey_id"`
		PaymentID   string `json:"payment_id"`
		Status      string `json:"status"`
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(j2RespBytes, &j2Resp)
	fmt.Printf("  ✅ Journey Created: ID=%s, PaymentID=%s, Status=%s\n", j2Resp.JourneyID, j2Resp.PaymentID, j2Resp.Status)

	// Parse session_id from redirect URL
	u, _ := url.Parse(j2Resp.RedirectURL)
	mfsSessionID := u.Query().Get("session_id")
	fmt.Printf("  ✅ Hosted Checkout Session ID: %s\n", mfsSessionID)

	// Complete checkout on mock-mfs
	fmt.Println("  ==> Submitting customer PIN/OTP at mock-mfs...")
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	form := url.Values{}
	form.Set("session_id", mfsSessionID)
	form.Set("msisdn", "01712345678")
	form.Set("pin", "1234")
	form.Set("otp", "123456")
	form.Set("scenario", "SUCCESS")
	mfsReq, _ := http.NewRequest(http.MethodPost, "http://localhost:5011/checkout/confirm", strings.NewReader(form.Encode()))
	mfsReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mfsResp, err := noRedirectClient.Do(mfsReq)
	if err != nil {
		fmt.Printf("FAIL: mock-mfs checkout confirm: %v\n", err)
		os.Exit(1)
	}
	mfsResp.Body.Close()

	// Poll orchestrator until journey is COMPLETED
	fmt.Println("  ==> Waiting for outbox event delivery and downstream recharge execution...")
	var j2Data Journey
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		status, jBytes, err := client.Do(ctx, http.MethodGet, "orchestrator", fmt.Sprintf("http://localhost:4010/journeys/%s", j2Resp.JourneyID), nil, nil)
		if err == nil && status == 200 {
			_ = json.Unmarshal(jBytes, &j2Data)
			if j2Data.Status == "COMPLETED" {
				break
			}
		}
	}
	fmt.Printf("  ✅ Journey Final Status: %s\n", j2Data.Status)
	for _, st := range j2Data.Steps {
		ref := ""
		if st.DownstreamReference != nil {
			ref = *st.DownstreamReference
		}
		fmt.Printf("    - Step %d [%s]: Status=%s, Attempts=%d, Ref=%s\n", st.StepOrder, st.StepName, st.Status, st.AttemptCount, ref)
	}
	if j2Data.Status != "COMPLETED" {
		fmt.Printf("FAIL: Expected journey COMPLETED, got %s\n", j2Data.Status)
		os.Exit(1)
	}

	// 5.3 Multi-step saga with downstream FAILURE & retries -> NEEDS_ATTENTION
	fmt.Println("\n==> 5.3 Starting 'payment_recharge' journey with Downstream FAILURE (NEEDS_ATTENTION test)...")
	journey3Req := map[string]any{
		"journey_type":        "payment_recharge",
		"customer_id":         "cust_saga_user",
		"amount":              10000,
		"currency":            "BDT",
		"method":              "mfs",
		"provider":            "mock-mfs",
		"mobile_number":       "01799887766",
		"downstream_scenario": "FAIL",
		"return_url":          "http://localhost:3010/return",
	}
	j3Bytes, _ := json.Marshal(journey3Req)
	status, j3RespBytes, err := client.Post(ctx, "orchestrator", "http://localhost:4010/journeys", j3Bytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Start fail journey failed: %v (%s)\n", err, string(j3RespBytes))
		os.Exit(1)
	}
	var j3Resp struct {
		JourneyID   string `json:"journey_id"`
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(j3RespBytes, &j3Resp)
	u3, _ := url.Parse(j3Resp.RedirectURL)
	mfsSessionID3 := u3.Query().Get("session_id")

	// Complete payment successfully at mock-mfs
	form3 := url.Values{}
	form3.Set("session_id", mfsSessionID3)
	form3.Set("msisdn", "01799887766")
	form3.Set("pin", "1234")
	form3.Set("otp", "123456")
	form3.Set("scenario", "SUCCESS")
	mfsReq3, _ := http.NewRequest(http.MethodPost, "http://localhost:5011/checkout/confirm", strings.NewReader(form3.Encode()))
	mfsReq3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mfsResp3, _ := noRedirectClient.Do(mfsReq3)
	mfsResp3.Body.Close()

	fmt.Println("  ==> Waiting for downstream retries (attempt 1, 2, 3) to exhaust...")
	var j3Data Journey
	for i := 0; i < 20; i++ {
		time.Sleep(1 * time.Second)
		status, jBytes, err := client.Do(ctx, http.MethodGet, "orchestrator", fmt.Sprintf("http://localhost:4010/journeys/%s", j3Resp.JourneyID), nil, nil)
		if err == nil && status == 200 {
			_ = json.Unmarshal(jBytes, &j3Data)
			if j3Data.Status == "NEEDS_ATTENTION" {
				break
			}
		}
	}
	fmt.Printf("  ✅ Journey Status: %s (Expected: NEEDS_ATTENTION)\n", j3Data.Status)
	for _, st := range j3Data.Steps {
		errStr := ""
		if st.LastError != nil {
			errStr = *st.LastError
		}
		fmt.Printf("    - Step %d [%s]: Status=%s, Attempts=%d, Error='%s'\n", st.StepOrder, st.StepName, st.Status, st.AttemptCount, errStr)
	}
	if j3Data.Status != "NEEDS_ATTENTION" {
		fmt.Printf("FAIL: Expected journey status NEEDS_ATTENTION, got %s\n", j3Data.Status)
		os.Exit(1)
	}

	// 5.4 Manual Retry API
	fmt.Println("\n==> 5.4 Testing Manual Retry endpoint POST /journeys/{id}/steps/RECHARGE/retry...")
	retryURL := fmt.Sprintf("http://localhost:4010/journeys/%s/steps/RECHARGE/retry", j3Resp.JourneyID)
	status, retryResp, err := client.Post(ctx, "orchestrator", retryURL, []byte("{}"), nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Manual retry failed: %v (%s)\n", err, string(retryResp))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Manual retry successfully triggered: %s\n", string(retryResp))

	fmt.Println("\n==========================================================")
	fmt.Println(" 🎉 ALL PHASE 4 & PHASE 5 TESTS PASSED SUCCESSFULLY!     ")
	fmt.Println("==========================================================")
}
