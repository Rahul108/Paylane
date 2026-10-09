package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"orchestrator/internal/model"
	"orchestrator/internal/saga"
	"paylane-jwe"
)

var (
	ErrJourneyNotFound = errors.New("journey not found")
	ErrStepNotFound    = errors.New("journey step not found")
)

type StartJourneyRequest struct {
	JourneyType    string          `json:"journey_type"`
	CustomerID     string          `json:"customer_id"`
	Amount         int64           `json:"amount"` // subunits
	Currency       string          `json:"currency"`
	Method         string          `json:"method"`   // mfs, card
	Provider       string          `json:"provider"` // mock-mfs, mock-card
	TokenReference string          `json:"token_reference,omitempty"`
	MobileNumber   string          `json:"mobile_number,omitempty"`
	PromoCode      string          `json:"promo_code,omitempty"`
	PlanName       string          `json:"plan_name,omitempty"`
	DownstreamScen string          `json:"downstream_scenario,omitempty"`
	ReturnURL      string          `json:"return_url,omitempty"`
}

type StartJourneyResponse struct {
	JourneyID   string `json:"journey_id"`
	PaymentID   string `json:"payment_id,omitempty"`
	Status      string `json:"status"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

type Service struct {
	db             *sql.DB
	jweClient      *jwe.Client
	paymentCoreURL string
	downstreamURL  string
	logger         *slog.Logger
}

func NewService(db *sql.DB, jweClient *jwe.Client, paymentCoreURL, downstreamURL string, logger *slog.Logger) *Service {
	if paymentCoreURL == "" {
		paymentCoreURL = "http://payment-core:4011"
	}
	if downstreamURL == "" {
		downstreamURL = "http://mock-downstream:5013"
	}
	return &Service{
		db:             db,
		jweClient:      jweClient,
		paymentCoreURL: paymentCoreURL,
		downstreamURL:  downstreamURL,
		logger:         logger,
	}
}

// StartJourney creates journey and steps, calls payment-core, and starts execution.
func (s *Service) StartJourney(ctx context.Context, req StartJourneyRequest) (*StartJourneyResponse, error) {
	steps, err := saga.GetSteps(req.JourneyType)
	if err != nil {
		return nil, err
	}

	journeyID := model.NewULID()
	now := time.Now().UTC()

	inputBytes, _ := json.Marshal(req)

	// 1. Insert Journey row in STARTED status
	insertJourney := `
		INSERT INTO journeys (id, journey_type, customer_id, status, input_payload, current_step, created_at, updated_at) 
		VALUES (?, ?, ?, 'STARTED', ?, 'PAYMENT', ?, ?)`
	_, err = s.db.ExecContext(ctx, insertJourney, journeyID, req.JourneyType, req.CustomerID, inputBytes, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create journey: %w", err)
	}

	// 2. Insert ordered step rows
	for i, stepName := range steps {
		stepID := model.NewULID()
		insertStep := `
			INSERT INTO journey_steps (id, journey_id, step_name, step_order, status, attempt_count, created_at, updated_at) 
			VALUES (?, ?, ?, ?, 'PENDING', 0, ?, ?)`
		_, _ = s.db.ExecContext(ctx, insertStep, stepID, journeyID, stepName, i+1, now, now)
	}

	// 3. Execute Step 1: PAYMENT
	idempotencyKey := fmt.Sprintf("%s_PAYMENT", journeyID)
	payReq := map[string]any{
		"idempotency_key": idempotencyKey,
		"customer_id":     req.CustomerID,
		"amount":          req.Amount,
		"currency":        req.Currency,
		"method":          req.Method,
		"provider":        req.Provider,
		"token_reference": req.TokenReference,
		"return_url":      req.ReturnURL,
	}
	payReqBytes, _ := json.Marshal(payReq)

	var payResp struct {
		Payment struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"payment"`
		RedirectURL string `json:"redirect_url"`
		SessionID   string `json:"session_id"`
	}

	code, respBytes, err := s.jweClient.Post(ctx, "payment-core", s.paymentCoreURL+"/payments", payReqBytes, nil)
	if err != nil || (code != 200 && code != 201) {
		failReason := fmt.Sprintf("payment-core call failed (%d): %s", code, string(respBytes))
		s.logger.Error("payment initiation failed", "err", failReason)
		_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusFailed, nil)
		_ = s.updateStep(ctx, journeyID, "PAYMENT", model.StepStatusFailed, 1, &failReason, nil)
		return nil, errors.New(failReason)
	}

	_ = json.Unmarshal(respBytes, &payResp)
	paymentID := payResp.Payment.ID

	// Link payment_id to journey
	updatePaymentLink := `UPDATE journeys SET payment_id = ? WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, updatePaymentLink, paymentID, journeyID)

	if req.TokenReference != "" && payResp.Payment.Status == "CAPTURED" {
		// UI-LESS DIRECT FLOW: Payment was captured synchronously!
		_ = s.updateStep(ctx, journeyID, "PAYMENT", model.StepStatusSucceeded, 1, nil, nil)

		if len(steps) == 1 {
			// No downstream steps: completed!
			_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusCompleted, nil)
			return &StartJourneyResponse{
				JourneyID: journeyID,
				PaymentID: paymentID,
				Status:    string(model.JourneyStatusCompleted),
			}, nil
		}

		// Advance to downstream steps
		_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusRunningSteps, &steps[1])
		go s.executeDownstream(context.Background(), journeyID, steps[1], req)

		return &StartJourneyResponse{
			JourneyID: journeyID,
			PaymentID: paymentID,
			Status:    string(model.JourneyStatusRunningSteps),
		}, nil
	}

	// Standard UI Hosted flow: Awaiting payment via hosted checkout
	_ = s.updateStep(ctx, journeyID, "PAYMENT", model.StepStatusRunning, 1, nil, nil)
	_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusAwaitingPayment, nil)

	return &StartJourneyResponse{
		JourneyID:   journeyID,
		PaymentID:   paymentID,
		Status:      string(model.JourneyStatusAwaitingPayment),
		RedirectURL: payResp.RedirectURL,
	}, nil
}

// HandleEvent receives outbox events from payment-core over JWE.
func (s *Service) HandleEvent(ctx context.Context, eventType, aggregateID string, payload map[string]any) error {
	s.logger.Info("received outbox event from payment-core", "type", eventType, "payment_id", aggregateID)

	var journeyID, journeyType string
	var inputBytes []byte
	query := `SELECT id, journey_type, input_payload FROM journeys WHERE payment_id = ?`
	err := s.db.QueryRowContext(ctx, query, aggregateID).Scan(&journeyID, &journeyType, &inputBytes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.logger.Warn("no journey found matching payment_id", "payment_id", aggregateID)
			return nil
		}
		return err
	}

	var req StartJourneyRequest
	_ = json.Unmarshal(inputBytes, &req)

	switch eventType {
	case "payment.captured":
		_ = s.updateStep(ctx, journeyID, "PAYMENT", model.StepStatusSucceeded, 1, nil, nil)
		steps, _ := saga.GetSteps(journeyType)

		if len(steps) == 1 {
			// Single-step journey: completed!
			_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusCompleted, nil)
			s.logger.Info("journey completed successfully", "journey_id", journeyID)
		} else {
			// Multi-step saga: trigger downstream execution
			nextStep := steps[1]
			_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusRunningSteps, &nextStep)
			go s.executeDownstream(context.Background(), journeyID, nextStep, req)
		}

	case "payment.failed", "payment.expired":
		failErr := "Payment declined or expired"
		_ = s.updateStep(ctx, journeyID, "PAYMENT", model.StepStatusFailed, 1, &failErr, nil)
		_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusFailed, nil)
	}

	return nil
}

func (s *Service) executeDownstream(ctx context.Context, journeyID, stepName string, req StartJourneyRequest) {
	s.logger.Info("executing downstream step", "journey_id", journeyID, "step", stepName)

	idempotencyKey := fmt.Sprintf("%s_%s", journeyID, stepName)
	maxRetries := 3
	var lastErr string

	for attempt := 1; attempt <= maxRetries; attempt++ {
		_ = s.updateStep(ctx, journeyID, stepName, model.StepStatusRunning, attempt, nil, nil)

		var endpoint string
		var body map[string]any

		switch stepName {
		case "RECHARGE":
			endpoint = "/recharge"
			body = map[string]any{
				"idempotency_key": idempotencyKey,
				"customer_id":     req.CustomerID,
				"mobile_number":   req.MobileNumber,
				"amount":          req.Amount,
				"scenario":        req.DownstreamScen,
			}
		case "CASHBACK":
			endpoint = "/cashback"
			body = map[string]any{
				"idempotency_key": idempotencyKey,
				"customer_id":     req.CustomerID,
				"promo_code":      req.PromoCode,
				"amount":          req.Amount / 10,
				"scenario":        req.DownstreamScen,
			}
		case "SUBSCRIBE":
			endpoint = "/subscribe"
			body = map[string]any{
				"idempotency_key": idempotencyKey,
				"customer_id":     req.CustomerID,
				"plan_name":       req.PlanName,
				"scenario":        req.DownstreamScen,
			}
		default:
			return
		}

		bBytes, _ := json.Marshal(body)
		code, respBytes, err := s.jweClient.Post(ctx, "mock-downstream", s.downstreamURL+endpoint, bBytes, nil)

		if err == nil && code == 200 {
			// Downstream step SUCCEEDED!
			var res struct {
				ReferenceID string `json:"reference_id"`
			}
			_ = json.Unmarshal(respBytes, &res)

			_ = s.updateStep(ctx, journeyID, stepName, model.StepStatusSucceeded, attempt, nil, &res.ReferenceID)
			_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusCompleted, nil)
			s.logger.Info("downstream step succeeded", "journey_id", journeyID, "ref", res.ReferenceID)
			return
		}

		lastErr = fmt.Sprintf("downstream error (HTTP %d): %s", code, string(respBytes))
		s.logger.Warn("downstream step attempt failed", "attempt", attempt, "err", lastErr)

		if attempt < maxRetries {
			_ = s.updateStep(ctx, journeyID, stepName, model.StepStatusRetrying, attempt, &lastErr, nil)
			time.Sleep(time.Duration(attempt) * time.Second) // backoff
		}
	}

	// Exhausted retries: Money was deducted, service was NOT delivered!
	// Mark step and journey as NEEDS_ATTENTION!
	s.logger.Error("downstream retries exhausted! Flagging NEEDS_ATTENTION", "journey_id", journeyID, "step", stepName)
	_ = s.updateStep(ctx, journeyID, stepName, model.StepStatusNeedsAttention, maxRetries, &lastErr, nil)
	_ = s.updateJourneyStatus(ctx, journeyID, model.JourneyStatusNeedsAttention, &stepName)
}

func (s *Service) GetJourney(ctx context.Context, id string) (*model.Journey, error) {
	query := `
		SELECT id, journey_type, customer_id, status, input_payload, current_step, payment_id, created_at, updated_at 
		FROM journeys 
		WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var j model.Journey
	var statusStr string
	var currStep, payID sql.NullString
	var inputBytes []byte

	if err := row.Scan(&j.ID, &j.JourneyType, &j.CustomerID, &statusStr, &inputBytes, &currStep, &payID, &j.CreatedAt, &j.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJourneyNotFound
		}
		return nil, err
	}

	j.Status = model.JourneyStatus(statusStr)
	j.InputPayload = inputBytes
	if currStep.Valid {
		j.CurrentStep = &currStep.String
	}
	if payID.Valid {
		j.PaymentID = &payID.String
	}

	// Fetch steps
	stepQuery := `
		SELECT id, journey_id, step_name, step_order, status, attempt_count, last_error, downstream_reference, request_summary, response_summary, created_at, updated_at 
		FROM journey_steps 
		WHERE journey_id = ? 
		ORDER BY step_order ASC`
	rows, err := s.db.QueryContext(ctx, stepQuery, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var st model.JourneyStep
			var stStatus string
			var lastErr, downRef sql.NullString
			var reqSum, respSum []byte
			if scanErr := rows.Scan(&st.ID, &st.JourneyID, &st.StepName, &st.StepOrder, &stStatus, &st.AttemptCount, &lastErr, &downRef, &reqSum, &respSum, &st.CreatedAt, &st.UpdatedAt); scanErr == nil {
				st.Status = model.StepStatus(stStatus)
				if lastErr.Valid {
					st.LastError = &lastErr.String
				}
				if downRef.Valid {
					st.DownstreamReference = &downRef.String
				}
				st.RequestSummary = reqSum
				st.ResponseSummary = respSum
				j.Steps = append(j.Steps, st)
			}
		}
	}

	return &j, nil
}

func (s *Service) RetryStep(ctx context.Context, journeyID, stepName string) error {
	j, err := s.GetJourney(ctx, journeyID)
	if err != nil {
		return err
	}
	var req StartJourneyRequest
	_ = json.Unmarshal(j.InputPayload, &req)

	go s.executeDownstream(context.Background(), journeyID, stepName, req)
	return nil
}

func (s *Service) updateJourneyStatus(ctx context.Context, journeyID string, status model.JourneyStatus, currentStep *string) error {
	now := time.Now().UTC()
	query := `UPDATE journeys SET status = ?, current_step = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, string(status), currentStep, now, journeyID)
	return err
}

func (s *Service) updateStep(ctx context.Context, journeyID, stepName string, status model.StepStatus, attempts int, lastErr *string, ref *string) error {
	now := time.Now().UTC()
	query := `
		UPDATE journey_steps 
		SET status = ?, attempt_count = ?, last_error = ?, downstream_reference = ?, updated_at = ? 
		WHERE journey_id = ? AND step_name = ?`
	_, err := s.db.ExecContext(ctx, query, string(status), attempts, lastErr, ref, now, journeyID, stepName)
	return err
}
