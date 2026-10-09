package service_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"mock-mfs/internal/service"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "mock_mfs_user:mock_mfs_pass@tcp(127.0.0.1:3306)/paylane_mock_mfs?parseTime=true&loc=UTC&charset=utf8mb4"
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

func TestMFS_CreateSessionAndConfirm(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	svc := service.NewService(db, nil, "http://localhost:5011", nil)
	ctx := context.Background()

	// 1. Create session
	sessResp, err := svc.CreateSession(ctx, service.CreateSessionRequest{
		PaymentID:   "01JMFS_TEST_001",
		Amount:      10000,
		Currency:    "BDT",
		CallbackURL: "http://payment-core:4011/webhooks/mock-mfs",
		ReturnURL:   "http://localhost:3010/return",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if sessResp.SessionID == "" || !strings.Contains(sessResp.RedirectURL, "checkout") {
		t.Fatalf("invalid session response: %+v", sessResp)
	}

	// 2. Query session
	sess, err := svc.GetSession(ctx, sessResp.SessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if sess.Status != "PENDING" || sess.Amount != 10000 {
		t.Fatalf("unexpected session data: %+v", sess)
	}

	// 3. Confirm with valid PIN and OTP
	returnURL, err := svc.ConfirmPayment(ctx, sessResp.SessionID, "01700000000", "1234", "123456", "SUCCESS")
	if err != nil {
		t.Fatalf("failed to confirm payment: %v", err)
	}
	if !strings.Contains(returnURL, "status=success") {
		t.Fatalf("expected return URL with status=success, got %s", returnURL)
	}

	// 4. Verify updated status
	sessAfter, _ := svc.GetSession(ctx, sessResp.SessionID)
	if sessAfter.Status != "SUCCESS" {
		t.Fatalf("expected session status SUCCESS, got %s", sessAfter.Status)
	}

	// 5. Test Insufficient Funds scenario
	sessFailResp, err := svc.CreateSession(ctx, service.CreateSessionRequest{
		PaymentID:   "01JMFS_TEST_FAIL",
		Amount:      50000,
		Currency:    "BDT",
		ReturnURL:   "http://localhost:3010/return",
	})
	if err != nil {
		t.Fatalf("failed to create fail session: %v", err)
	}
	returnFailURL, err := svc.ConfirmPayment(ctx, sessFailResp.SessionID, "01700000000", "1234", "123456", "INSUFFICIENT_FUNDS")
	if err != nil {
		t.Fatalf("failed to process fail payment: %v", err)
	}
	if !strings.Contains(returnFailURL, "status=failed") {
		t.Fatalf("expected return URL with status=failed, got %s", returnFailURL)
	}
}
