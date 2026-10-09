package service_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"orchestrator/internal/model"
	"orchestrator/internal/service"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "orchestrator_user:orchestrator_pass@tcp(127.0.0.1:3306)/paylane_orchestrator?parseTime=true&loc=UTC&charset=utf8mb4"
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

func TestOrchestrator_JourneyModelAndSteps(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	svc := service.NewService(db, nil, "http://payment-core:4011", "http://mock-downstream:5013", nil)
	ctx := context.Background()

	// Insert test journey directly into MySQL
	journeyID := model.NewULID()
	now := "2026-10-10 00:00:00"
	_, err := db.ExecContext(ctx, `
		INSERT INTO journeys (id, journey_type, customer_id, status, input_payload, created_at, updated_at) 
		VALUES (?, 'payment_recharge', 'cust_01', 'STARTED', '{}', ?, ?)`, journeyID, now, now)
	if err != nil {
		t.Fatalf("failed to insert journey: %v", err)
	}

	// Insert step
	stepID := model.NewULID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO journey_steps (id, journey_id, step_name, step_order, status, attempt_count, created_at, updated_at) 
		VALUES (?, ?, 'PAYMENT', 1, 'PENDING', 0, ?, ?)`, stepID, journeyID, now, now)
	if err != nil {
		t.Fatalf("failed to insert step: %v", err)
	}

	// Retrieve journey
	j, err := svc.GetJourney(ctx, journeyID)
	if err != nil {
		t.Fatalf("failed to get journey: %v", err)
	}
	if j.ID != journeyID || j.JourneyType != "payment_recharge" {
		t.Fatalf("unexpected journey data: %+v", j)
	}
	if len(j.Steps) != 1 || j.Steps[0].StepName != "PAYMENT" {
		t.Fatalf("unexpected step data: %+v", j.Steps)
	}
}
