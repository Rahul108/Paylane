package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"mock-downstream/internal/model"
)

var (
	ErrCallFailed = errors.New("downstream delivery failed")
)

type RechargeRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CustomerID     string `json:"customer_id"`
	MobileNumber   string `json:"mobile_number"`
	Amount         int64  `json:"amount"`
	Operator       string `json:"operator,omitempty"`
	Scenario       string `json:"scenario,omitempty"`
}

type RechargeResponse struct {
	ReferenceID string `json:"reference_id"`
	Status      string `json:"status"` // SUCCEEDED, FAILED
	Message     string `json:"message"`
}

type CashbackRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CustomerID     string `json:"customer_id"`
	PromoCode      string `json:"promo_code"`
	Amount         int64  `json:"amount"`
	Scenario       string `json:"scenario,omitempty"`
}

type CashbackResponse struct {
	ReferenceID string `json:"reference_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

type SubscribeRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CustomerID     string `json:"customer_id"`
	PlanName       string `json:"plan_name"`
	Scenario       string `json:"scenario,omitempty"`
}

type SubscribeResponse struct {
	ReferenceID string `json:"reference_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

type Service struct {
	db        *sql.DB
	attemptMu sync.Mutex
	attempts  map[string]int
}

func NewService(db *sql.DB) *Service {
	return &Service{
		db:       db,
		attempts: make(map[string]int),
	}
}

func (s *Service) ExecuteCall(ctx context.Context, callType, idempotencyKey, customerID string, amount *int64, scenario string) (string, error) {
	s.attemptMu.Lock()
	count := s.attempts[idempotencyKey]
	s.attempts[idempotencyKey] = count + 1
	attemptNum := count + 1
	s.attemptMu.Unlock()

	// 1. Idempotency Check
	var existingID, existingStatus string
	checkQ := `SELECT id, status FROM downstream_calls WHERE idempotency_key = ?`
	err := s.db.QueryRowContext(ctx, checkQ, idempotencyKey).Scan(&existingID, &existingStatus)
	if err == nil {
		if existingStatus == "SUCCEEDED" {
			// Idempotent duplicate: return already succeeded reference
			return fmt.Sprintf("REF_%s", existingID), nil
		}
	}

	// 2. Scenario simulation
	var status = "SUCCEEDED"
	var returnErr error

	switch strings.ToUpper(scenario) {
	case "FAIL":
		status = "FAILED"
		returnErr = fmt.Errorf("%w: downstream service declined call", ErrCallFailed)
	case "FAIL_THEN_SUCCEED":
		if attemptNum == 1 {
			status = "FAILED"
			returnErr = fmt.Errorf("%w: temporary downstream network error (attempt 1)", ErrCallFailed)
		} else {
			status = "SUCCEEDED"
		}
	case "TIMEOUT":
		status = "FAILED"
		returnErr = fmt.Errorf("%w: downstream request timed out", ErrCallFailed)
	default:
		status = "SUCCEEDED"
	}

	now := time.Now().UTC()
	callID := model.NewULID()

	// Record call
	insertQ := `
		INSERT INTO downstream_calls (id, call_type, idempotency_key, customer_id, amount, status, scenario_override, created_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE status = VALUES(status)`
	
	var scen sql.NullString
	if scenario != "" {
		scen = sql.NullString{String: scenario, Valid: true}
	}

	_, _ = s.db.ExecContext(ctx, insertQ, callID, callType, idempotencyKey, customerID, amount, status, scen, now)

	if returnErr != nil {
		return "", returnErr
	}

	return fmt.Sprintf("REF_%s", callID), nil
}

func (s *Service) Recharge(ctx context.Context, req RechargeRequest) (*RechargeResponse, error) {
	ref, err := s.ExecuteCall(ctx, "recharge", req.IdempotencyKey, req.CustomerID, &req.Amount, req.Scenario)
	if err != nil {
		return nil, err
	}
	return &RechargeResponse{
		ReferenceID: ref,
		Status:      "SUCCEEDED",
		Message:     fmt.Sprintf("Recharge of %d completed", req.Amount),
	}, nil
}

func (s *Service) Cashback(ctx context.Context, req CashbackRequest) (*CashbackResponse, error) {
	ref, err := s.ExecuteCall(ctx, "cashback", req.IdempotencyKey, req.CustomerID, &req.Amount, req.Scenario)
	if err != nil {
		return nil, err
	}
	return &CashbackResponse{
		ReferenceID: ref,
		Status:      "SUCCEEDED",
		Message:     fmt.Sprintf("Cashback of %d granted under promo %s", req.Amount, req.PromoCode),
	}, nil
}

func (s *Service) Subscribe(ctx context.Context, req SubscribeRequest) (*SubscribeResponse, error) {
	ref, err := s.ExecuteCall(ctx, "subscription", req.IdempotencyKey, req.CustomerID, nil, req.Scenario)
	if err != nil {
		return nil, err
	}
	return &SubscribeResponse{
		ReferenceID: ref,
		Status:      "SUCCEEDED",
		Message:     fmt.Sprintf("Subscription to plan %s activated", req.PlanName),
	}, nil
}
