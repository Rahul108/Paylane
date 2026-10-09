package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	_ "github.com/go-sql-driver/mysql"

	"payment-core/internal/adapter"
	"payment-core/internal/model"
	"payment-core/internal/service"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "payment_core_user:payment_core_pass@tcp(127.0.0.1:3306)/paylane_payment_core?parseTime=true&loc=UTC&charset=utf8mb4"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("skipping live database test: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping live database test (MySQL not reachable): %v", err)
	}
	return db
}

func TestPaymentService_FullLifecycle(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	adapters := adapter.NewRegistry()
	adapters.Register("mock-mfs", adapter.NewMFSAdapter("http://mock-mfs:5011"))
	adapters.Register("mfs", adapter.NewMFSAdapter("http://mock-mfs:5011"))

	svc := service.NewPaymentService(db, adapters)
	ctx := context.Background()

	idempotencyKey := "idem_" + model.NewULID()

	// 1. Initiate Payment
	initReq := service.InitiatePaymentRequest{
		IdempotencyKey: idempotencyKey,
		CustomerID:     "cust_test_01",
		Amount:         50000, // 500.00 BDT
		Currency:       "BDT",
		Method:         "mfs",
		Provider:       "mock-mfs",
		ReturnURL:      "http://localhost:3010/return",
	}

	resp1, err := svc.InitiatePayment(ctx, initReq)
	if err != nil {
		t.Fatalf("failed to initiate payment: %v", err)
	}
	if resp1.Payment.Status != model.StatusPending {
		t.Fatalf("expected status PENDING, got %s", resp1.Payment.Status)
	}
	paymentID := resp1.Payment.ID

	// 2. Test Idempotency: exact same call returns existing payment without creating new row
	resp2, err := svc.InitiatePayment(ctx, initReq)
	if err != nil {
		t.Fatalf("idempotent call failed: %v", err)
	}
	if resp2.Payment.ID != paymentID {
		t.Fatalf("expected same payment ID %s, got %s", paymentID, resp2.Payment.ID)
	}

	// 3. Webhook: Payment Captured
	webhookEventID := "wh_" + model.NewULID()
	rawWebhookPayload := json.RawMessage(`{"event":"PAYMENT.CAPTURED","provider":"mock-mfs"}`)
	err = svc.ProcessWebhook(ctx, "mock-mfs", webhookEventID, rawWebhookPayload, paymentID, "PAYMENT.CAPTURED", "PGW_CALLBACK_CAPTURED", "prov_ref_123")
	if err != nil {
		t.Fatalf("failed to process webhook: %v", err)
	}

	// Verify payment updated to CAPTURED
	pCaptured, err := svc.GetPayment(ctx, paymentID)
	if err != nil {
		t.Fatalf("failed to get payment: %v", err)
	}
	if pCaptured.Status != model.StatusCaptured {
		t.Fatalf("expected status CAPTURED, got %s", pCaptured.Status)
	}

	// 4. Test Webhook Deduplication: sending same webhook event ID again returns duplicate
	err = svc.ProcessWebhook(ctx, "mock-mfs", webhookEventID, rawWebhookPayload, paymentID, "PAYMENT.CAPTURED", "PGW_CALLBACK_CAPTURED", "prov_ref_123")
	if err != service.ErrDuplicateWebhook {
		t.Fatalf("expected ErrDuplicateWebhook, got %v", err)
	}

	// 5. Verify Double-Entry Ledger entries
	entries, err := svc.GetLedgerEntries(ctx, paymentID)
	if err != nil {
		t.Fatalf("failed to get ledger entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 ledger entries (1 debit, 1 credit), got %d", len(entries))
	}

	var totalDebit, totalCredit int64
	for _, entry := range entries {
		if entry.EntryType == model.LedgerDebit {
			totalDebit += entry.Amount
		} else if entry.EntryType == model.LedgerCredit {
			totalCredit += entry.Amount
		}
	}
	if totalDebit != 50000 || totalCredit != 50000 {
		t.Fatalf("ledger entries unbalanced: debit=%d credit=%d (expected 50000)", totalDebit, totalCredit)
	}

	// 6. Test Refund Processing & Ledger Reversal
	refundIdempotency := "ref_idem_" + model.NewULID()
	refund, err := svc.RefundPayment(ctx, paymentID, refundIdempotency, 50000, "CUSTOMER_CANCELLATION")
	if err != nil {
		t.Fatalf("failed to refund payment: %v", err)
	}
	if refund.Status != model.RefundStatusSucceeded {
		t.Fatalf("expected refund SUCCEEDED, got %s", refund.Status)
	}

	pRefunded, err := svc.GetPayment(ctx, paymentID)
	if err != nil {
		t.Fatalf("failed to get refunded payment: %v", err)
	}
	if pRefunded.Status != model.StatusRefunded {
		t.Fatalf("expected payment status REFUNDED, got %s", pRefunded.Status)
	}

	// Check reversing ledger entries (now total of 4 entries)
	allEntries, err := svc.GetLedgerEntries(ctx, paymentID)
	if err != nil {
		t.Fatalf("failed to get all ledger entries: %v", err)
	}
	if len(allEntries) != 4 {
		t.Fatalf("expected 4 total ledger entries (2 capture + 2 refund reversal), got %d", len(allEntries))
	}
	totalDebit = 0
	totalCredit = 0
	for _, entry := range allEntries {
		if entry.EntryType == model.LedgerDebit {
			totalDebit += entry.Amount
		} else if entry.EntryType == model.LedgerCredit {
			totalCredit += entry.Amount
		}
	}
	if totalDebit != totalCredit {
		t.Fatalf("ledger out of balance: totalDebit=%d totalCredit=%d", totalDebit, totalCredit)
	}

	// 7. Check audit events history
	events, err := svc.GetPaymentEvents(ctx, paymentID)
	if err != nil {
		t.Fatalf("failed to get payment events: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected at least 3 state transition events, got %d", len(events))
	}
}
