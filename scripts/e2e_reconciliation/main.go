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

type PaymentResp struct {
	Payment struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount int64  `json:"amount"`
	} `json:"payment"`
	RedirectURL string `json:"redirect_url"`
	SessionID   string `json:"session_id"`
}

type SinglePayment struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount int64  `json:"amount"`
}

type RefundResp struct {
	ID             string `json:"id"`
	PaymentID      string `json:"payment_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
}

type ReconcileItem struct {
	ID                string          `json:"id"`
	PaymentID         *string         `json:"payment_id,omitempty"`
	Provider          string          `json:"provider"`
	MismatchType      string          `json:"mismatch_type"`
	Details           json.RawMessage `json:"details"`
	ResolvedAt        *string         `json:"resolved_at,omitempty"`
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

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	fmt.Println("==========================================================")
	fmt.Println(" PHASE 6: TIMEOUTS, REFUNDS & RECONCILIATION VERIFICATION ")
	fmt.Println("==========================================================")

	// =========================================================================
	// 6.1 PARTIAL AND FULL REFUNDS
	// =========================================================================
	fmt.Println("\n==> 6.1 Testing Partial and Full Refunds + Ledger Reversals...")

	// 1. Create and capture a payment of 500.00 BDT (50000 subunits)
	initReq := map[string]any{
		"idempotency_key": fmt.Sprintf("p6_ref_%d", time.Now().UnixNano()),
		"customer_id":     "cust_p6_user",
		"amount":          50000,
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"return_url":      "http://localhost:3010/return",
	}
	initBytes, _ := json.Marshal(initReq)
	status, respBytes, err := client.Post(ctx, "payment-core", "http://localhost:4011/payments", initBytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Create payment failed: %v (%s)\n", err, string(respBytes))
		os.Exit(1)
	}
	var p1 PaymentResp
	_ = json.Unmarshal(respBytes, &p1)

	// Complete payment on mock-mfs
	form := url.Values{}
	form.Set("session_id", p1.SessionID)
	form.Set("msisdn", "01711223344")
	form.Set("pin", "1234")
	form.Set("otp", "123456")
	form.Set("scenario", "SUCCESS")
	req, _ := http.NewRequest(http.MethodPost, "http://localhost:5011/checkout/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := noRedirectClient.Do(req)
	resp.Body.Close()

	time.Sleep(1 * time.Second)

	// Check status is CAPTURED
	_, pCheckBytes, _ := client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", p1.Payment.ID), nil, nil)
	var pData SinglePayment
	_ = json.Unmarshal(pCheckBytes, &pData)
	if pData.Status != "CAPTURED" {
		fmt.Printf("FAIL: Expected CAPTURED, got %s\n", pData.Status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Payment Captured: ID=%s, Amount=%d BDT\n", pData.ID, pData.Amount)

	// 2. Partial Refund: 20000 BDT
	fmt.Println("  ==> Issuing Partial Refund of 20000 subunits (200 BDT)...")
	ref1Req := map[string]any{
		"idempotency_key": fmt.Sprintf("rfd1_%d", time.Now().UnixNano()),
		"amount":          20000,
		"reason":          "PARTIAL_PRODUCT_RETURN",
	}
	ref1Bytes, _ := json.Marshal(ref1Req)
	status, rfd1RespBytes, err := client.Post(ctx, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s/refunds", p1.Payment.ID), ref1Bytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Partial refund failed (status %d): %v (%s)\n", status, err, string(rfd1RespBytes))
		os.Exit(1)
	}
	var rfd1 RefundResp
	_ = json.Unmarshal(rfd1RespBytes, &rfd1)
	fmt.Printf("  ✅ Partial Refund 1 Created: ID=%s, Amount=%d, Status=%s\n", rfd1.ID, rfd1.Amount, rfd1.Status)

	// Assert payment remains CAPTURED after partial refund
	_, pCheckBytes, _ = client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", p1.Payment.ID), nil, nil)
	_ = json.Unmarshal(pCheckBytes, &pData)
	if pData.Status != "CAPTURED" {
		fmt.Printf("FAIL: Payment should remain CAPTURED after partial refund, got %s\n", pData.Status)
		os.Exit(1)
	}

	// Verify reversing ledger entries for 20000 exist
	_, ledgerBytes, _ := client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s/ledger", p1.Payment.ID), nil, nil)
	if !strings.Contains(string(ledgerBytes), `"amount":20000`) || !strings.Contains(string(ledgerBytes), "PARTIAL_REFUND") {
		fmt.Printf("FAIL: Reversing ledger entry for partial refund missing: %s\n", string(ledgerBytes))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Double-Entry Ledger recorded reversing entries for partial refund (20000 BDT)\n")

	// 3. Final Partial Refund: Remaining 30000 BDT
	fmt.Println("  ==> Issuing Remaining Refund of 30000 subunits (300 BDT)...")
	ref2Req := map[string]any{
		"idempotency_key": fmt.Sprintf("rfd2_%d", time.Now().UnixNano()),
		"amount":          30000,
		"reason":          "FULL_CANCELLATION",
	}
	ref2Bytes, _ := json.Marshal(ref2Req)
	status, rfd2RespBytes, err := client.Post(ctx, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s/refunds", p1.Payment.ID), ref2Bytes, nil)
	if err != nil || (status != 200 && status != 201) {
		fmt.Printf("FAIL: Final refund failed: %v (%s)\n", err, string(rfd2RespBytes))
		os.Exit(1)
	}
	var rfd2 RefundResp
	_ = json.Unmarshal(rfd2RespBytes, &rfd2)
	fmt.Printf("  ✅ Final Refund 2 Created: ID=%s, Amount=%d, Status=%s\n", rfd2.ID, rfd2.Amount, rfd2.Status)

	// Assert payment status is now REFUNDED
	_, pCheckBytes, _ = client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", p1.Payment.ID), nil, nil)
	_ = json.Unmarshal(pCheckBytes, &pData)
	if pData.Status != "REFUNDED" {
		fmt.Printf("FAIL: Expected payment status REFUNDED after 100%% refunds, got %s\n", pData.Status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Payment transitioned to terminal state: REFUNDED\n")

	// 4. Over-refund attempt rejection
	fmt.Println("  ==> Attempting over-refund on fully refunded payment...")
	refOverReq := map[string]any{
		"idempotency_key": fmt.Sprintf("rfd_over_%d", time.Now().UnixNano()),
		"amount":          5000,
	}
	refOverBytes, _ := json.Marshal(refOverReq)
	status, overResp, _ := client.Post(ctx, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s/refunds", p1.Payment.ID), refOverBytes, nil)
	if status == 200 || status == 201 {
		fmt.Printf("FAIL: Expected over-refund rejection, got success: %d\n", status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Over-refund correctly rejected! (HTTP status %d: %s)\n", status, string(overResp))

	// =========================================================================
	// 6.2 TIMEOUT SWEEP
	// =========================================================================
	fmt.Println("\n==> 6.2 Testing Payment Timeout Sweep (Stuck Payments Expired)...")

	// Create payment and leave in PENDING without paying
	initTimeoutReq := map[string]any{
		"idempotency_key": fmt.Sprintf("p6_timeout_%d", time.Now().UnixNano()),
		"customer_id":     "cust_p6_user",
		"amount":          15000,
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"return_url":      "http://localhost:3010/return",
	}
	initTBytes, _ := json.Marshal(initTimeoutReq)
	_, tRespBytes, _ := client.Post(ctx, "payment-core", "http://localhost:4011/payments", initTBytes, nil)
	var pTimeout PaymentResp
	_ = json.Unmarshal(tRespBytes, &pTimeout)
	fmt.Printf("  ✅ Created PENDING payment to test timeout: ID=%s\n", pTimeout.Payment.ID)

	time.Sleep(500 * time.Millisecond)

	// Trigger timeout sweep with max_age_seconds: 0 (immediate sweep)
	sweepReq := map[string]any{"max_age_seconds": 0}
	sweepBytes, _ := json.Marshal(sweepReq)
	status, sweepRespBytes, err := client.Post(ctx, "payment-core", "http://localhost:4011/timeouts/sweep", sweepBytes, nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Timeout sweep failed: %v (%s)\n", err, string(sweepRespBytes))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Timeout Sweep executed: %s\n", string(sweepRespBytes))

	// Verify payment is now EXPIRED
	_, pCheckBytes, _ = client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", pTimeout.Payment.ID), nil, nil)
	_ = json.Unmarshal(pCheckBytes, &pData)
	if pData.Status != "EXPIRED" {
		fmt.Printf("FAIL: Expected payment EXPIRED, got %s\n", pData.Status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Stuck payment successfully transitioned to EXPIRED!\n")

	// =========================================================================
	// 6.3 LATE WEBHOOK ON EXPIRED PAYMENT -> RECONCILIATION MISMATCH
	// =========================================================================
	fmt.Println("\n==> 6.3 Testing Late Webhook Callback for EXPIRED Payment...")

	// Customer completes payment now on mock-mfs for that expired session (late arrival)
	formLate := url.Values{}
	formLate.Set("session_id", pTimeout.SessionID)
	formLate.Set("msisdn", "01755667788")
	formLate.Set("pin", "1234")
	formLate.Set("otp", "123456")
	formLate.Set("scenario", "SUCCESS")
	reqLate, _ := http.NewRequest(http.MethodPost, "http://localhost:5011/checkout/confirm", strings.NewReader(formLate.Encode()))
	reqLate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	respLate, _ := noRedirectClient.Do(reqLate)
	respLate.Body.Close()

	time.Sleep(1 * time.Second)

	// Verify payment status is STILL EXPIRED (terminal state preserved)
	_, pCheckBytes, _ = client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", pTimeout.Payment.ID), nil, nil)
	_ = json.Unmarshal(pCheckBytes, &pData)
	if pData.Status != "EXPIRED" {
		fmt.Printf("FAIL: Expired payment should remain EXPIRED, got %s\n", pData.Status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Payment preserved terminal state EXPIRED (no illegal transition)\n")

	// Verify reconciliation item was recorded for LATE_SUCCESS_AFTER_EXPIRED
	status, reconItemsBytes, err := client.Do(ctx, http.MethodGet, "payment-core", "http://localhost:4011/reconciliation/items", nil, nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Query reconciliation items failed: %v (%s)\n", err, string(reconItemsBytes))
		os.Exit(1)
	}
	var reconList struct {
		Items []ReconcileItem `json:"items"`
	}
	_ = json.Unmarshal(reconItemsBytes, &reconList)

	foundLateMismatch := false
	for _, item := range reconList.Items {
		if item.PaymentID != nil && *item.PaymentID == pTimeout.Payment.ID && item.MismatchType == "LATE_SUCCESS_AFTER_EXPIRED" {
			foundLateMismatch = true
			fmt.Printf("  ✅ Reconciliation mismatch recorded: ID=%s, Type=%s\n", item.ID, item.MismatchType)
			break
		}
	}
	if !foundLateMismatch {
		fmt.Printf("FAIL: Expected LATE_SUCCESS_AFTER_EXPIRED mismatch in reconciliation_items, got: %s\n", string(reconItemsBytes))
		os.Exit(1)
	}

	// =========================================================================
	// 6.4 RECONCILIATION RUN & STATUS MISMATCH DETECTION
	// =========================================================================
	fmt.Println("\n==> 6.4 Testing Settlement Report Reconciliation & Discrepancy Detection...")

	// 1. Create payment with RECONCILIATION_MISMATCH (provider records success, but no callback is sent)
	initReconReq := map[string]any{
		"idempotency_key": fmt.Sprintf("p6_recon_%d", time.Now().UnixNano()),
		"customer_id":     "cust_p6_user",
		"amount":          35000,
		"currency":        "BDT",
		"method":          "mfs",
		"provider":        "mock-mfs",
		"return_url":      "http://localhost:3010/return",
	}
	initRBytes, _ := json.Marshal(initReconReq)
	_, rRespBytes, _ := client.Post(ctx, "payment-core", "http://localhost:4011/payments", initRBytes, nil)
	var pRecon PaymentResp
	_ = json.Unmarshal(rRespBytes, &pRecon)

	// Complete on mock-mfs with scenario RECONCILIATION_MISMATCH
	formRecon := url.Values{}
	formRecon.Set("session_id", pRecon.SessionID)
	formRecon.Set("msisdn", "01788990011")
	formRecon.Set("pin", "1234")
	formRecon.Set("otp", "123456")
	formRecon.Set("scenario", "RECONCILIATION_MISMATCH")
	reqRecon, _ := http.NewRequest(http.MethodPost, "http://localhost:5011/checkout/confirm", strings.NewReader(formRecon.Encode()))
	reqRecon.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	respRecon, _ := noRedirectClient.Do(reqRecon)
	respRecon.Body.Close()

	// Verify payment-core still sees PENDING (never received callback)
	_, pCheckBytes, _ = client.Do(ctx, http.MethodGet, "payment-core", fmt.Sprintf("http://localhost:4011/payments/%s", pRecon.Payment.ID), nil, nil)
	_ = json.Unmarshal(pCheckBytes, &pData)
	if pData.Status != "PENDING" {
		fmt.Printf("FAIL: Expected PENDING before reconciliation, got %s\n", pData.Status)
		os.Exit(1)
	}
	fmt.Printf("  ✅ Created discrepancy: Provider recorded SUCCESS, payment-core is %s\n", pData.Status)

	// 2. Run Reconciliation job
	fmt.Println("  ==> Running reconciliation job against provider settlement report...")
	status, reconRunBytes, err := client.Post(ctx, "payment-core", "http://localhost:4011/reconciliation/run", []byte(`{"provider":"mock-mfs"}`), nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Reconciliation run failed: %v (%s)\n", err, string(reconRunBytes))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Reconciliation Report: %s\n", string(reconRunBytes))

	// 3. Query reconciliation items and verify STATUS_MISMATCH was logged
	_, reconItemsBytes, _ = client.Do(ctx, http.MethodGet, "payment-core", "http://localhost:4011/reconciliation/items?unresolved=true", nil, nil)
	_ = json.Unmarshal(reconItemsBytes, &reconList)

	var mismatchItemID string
	for _, item := range reconList.Items {
		if item.PaymentID != nil && *item.PaymentID == pRecon.Payment.ID && item.MismatchType == "STATUS_MISMATCH" {
			mismatchItemID = item.ID
			fmt.Printf("  ✅ Discrepancy successfully detected: ID=%s, Type=%s, Details=%s\n", item.ID, item.MismatchType, string(item.Details))
			break
		}
	}
	if mismatchItemID == "" {
		fmt.Printf("FAIL: Expected STATUS_MISMATCH for payment %s, got: %s\n", pRecon.Payment.ID, string(reconItemsBytes))
		os.Exit(1)
	}

	// 4. Resolve the reconciliation mismatch item
	fmt.Println("  ==> Resolving reconciliation discrepancy...")
	resolveURL := fmt.Sprintf("http://localhost:4011/reconciliation/items/%s/resolve", mismatchItemID)
	status, resolveBytes, err := client.Post(ctx, "payment-core", resolveURL, []byte("{}"), nil)
	if err != nil || status != 200 {
		fmt.Printf("FAIL: Resolve mismatch failed: %v (%s)\n", err, string(resolveBytes))
		os.Exit(1)
	}
	fmt.Printf("  ✅ Reconciliation mismatch resolved successfully: %s\n", string(resolveBytes))

	fmt.Println("\n==========================================================")
	fmt.Println(" 🎉 ALL PHASE 6 TESTS PASSED SUCCESSFULLY!                ")
	fmt.Println("==========================================================")
}
