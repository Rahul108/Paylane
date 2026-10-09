package service_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"mock-card/internal/service"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "mock_card_user:mock_card_pass@tcp(127.0.0.1:3306)/paylane_mock_card?parseTime=true&loc=UTC&charset=utf8mb4"
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

func TestCard_CreateSessionAndConfirm(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	svc := service.NewService(db, nil, "http://localhost:5012", nil)
	ctx := context.Background()

	// 1. Create card session
	sessResp, err := svc.CreateSession(ctx, service.CreateSessionRequest{
		PaymentID:   "01JCRD_TEST_001",
		Amount:      25000,
		Currency:    "BDT",
		CallbackURL: "http://payment-core:4011/webhooks/mock-card",
		ReturnURL:   "http://localhost:3010/return",
	})
	if err != nil {
		t.Fatalf("failed to create card session: %v", err)
	}
	if sessResp.SessionID == "" || !strings.Contains(sessResp.RedirectURL, "checkout") {
		t.Fatalf("invalid session response: %+v", sessResp)
	}

	// 2. Visa card success
	returnURL, err := svc.ConfirmPayment(ctx, sessResp.SessionID, "Jane Doe", "4111111111111111", "12/28", "123", "123456", "SUCCESS")
	if err != nil {
		t.Fatalf("failed to confirm card payment: %v", err)
	}
	if !strings.Contains(returnURL, "status=success") {
		t.Fatalf("expected return URL with status=success, got %s", returnURL)
	}

	// 3. Test Insufficient Funds Card Number
	sessFailResp, err := svc.CreateSession(ctx, service.CreateSessionRequest{
		PaymentID:   "01JCRD_TEST_FAIL",
		Amount:      50000,
		Currency:    "BDT",
		ReturnURL:   "http://localhost:3010/return",
	})
	if err != nil {
		t.Fatalf("failed to create fail session: %v", err)
	}

	returnFailURL, err := svc.ConfirmPayment(ctx, sessFailResp.SessionID, "Jane Doe", "4000000000000002", "12/28", "123", "123456", "")
	if err != nil {
		t.Fatalf("failed to process fail card payment: %v", err)
	}
	if !strings.Contains(returnFailURL, "status=failed") {
		t.Fatalf("expected return URL with status=failed, got %s", returnFailURL)
	}
}
