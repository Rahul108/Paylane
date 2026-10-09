package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"payment-core/internal/adapter"
	"payment-core/internal/model"
	"payment-core/internal/statemachine"
)

var (
	ErrInvalidRequest   = errors.New("invalid payment request")
	ErrDuplicateWebhook = errors.New("duplicate webhook event already processed")
)

type InitiatePaymentRequest struct {
	IdempotencyKey string          `json:"idempotency_key"`
	CustomerID     string          `json:"customer_id"`
	Amount         int64           `json:"amount"` // in subunits (paisa)
	Currency       string          `json:"currency"`
	Method         string          `json:"method"`   // mfs, card
	Provider       string          `json:"provider"` // mock-mfs, mock-card
	TokenReference string          `json:"token_reference,omitempty"`
	ReturnURL      string          `json:"return_url,omitempty"`
	CallbackURL    string          `json:"callback_url,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type InitiatePaymentResponse struct {
	Payment     *model.Payment `json:"payment"`
	RedirectURL string         `json:"redirect_url,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
}

type PaymentService struct {
	db              *sql.DB
	adapterRegistry *adapter.Registry
}

func NewPaymentService(db *sql.DB, adapterRegistry *adapter.Registry) *PaymentService {
	return &PaymentService{
		db:              db,
		adapterRegistry: adapterRegistry,
	}
}

// InitiatePayment validates input, enforces idempotency, creates the payment record,
// calls the adapter, and transitions status.
func (s *PaymentService) InitiatePayment(ctx context.Context, req InitiatePaymentRequest) (*InitiatePaymentResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidRequest)
	}
	if req.CustomerID == "" {
		return nil, fmt.Errorf("%w: customer_id is required", ErrInvalidRequest)
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be greater than zero", ErrInvalidRequest)
	}
	if req.Currency == "" {
		req.Currency = "BDT"
	}
	if req.Method == "" {
		return nil, fmt.Errorf("%w: method is required (mfs, card)", ErrInvalidRequest)
	}
	if req.Provider == "" {
		if strings.EqualFold(req.Method, "mfs") {
			req.Provider = "mock-mfs"
		} else if strings.EqualFold(req.Method, "card") {
			req.Provider = "mock-card"
		} else {
			return nil, fmt.Errorf("%w: unknown provider for method %s", ErrInvalidRequest, req.Method)
		}
	}

	// 1. Idempotency Check: check if a payment with this idempotency key already exists
	existing, err := s.GetPaymentByIdempotencyKey(ctx, req.IdempotencyKey)
	if err == nil && existing != nil {
		// Found existing payment, return it directly
		return &InitiatePaymentResponse{
			Payment: existing,
		}, nil
	}

	adp, err := s.adapterRegistry.Get(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("provider adapter unavailable: %w", err)
	}

	// 2. Insert new payment row in CREATED state
	paymentID := model.NewULID()
	now := time.Now().UTC()

	metaJSON := req.Metadata
	if len(metaJSON) == 0 {
		metaJSON = json.RawMessage("{}")
	}

	insertQuery := `
		INSERT INTO payments (id, idempotency_key, customer_id, amount, currency, method, provider, status, metadata, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, insertQuery, paymentID, req.IdempotencyKey, req.CustomerID, req.Amount, req.Currency, req.Method, req.Provider, string(model.StatusCreated), metaJSON, now, now)
	if err != nil {
		// Check for race-condition duplicate key error
		if isDuplicateEntryError(err) {
			if existingPayment, fetchErr := s.GetPaymentByIdempotencyKey(ctx, req.IdempotencyKey); fetchErr == nil {
				return &InitiatePaymentResponse{Payment: existingPayment}, nil
			}
		}
		return nil, fmt.Errorf("failed to insert payment: %w", err)
	}

	// 3. Provider flow
	if req.TokenReference != "" {
		// UI-less payment using bound token
		chgResp, err := adp.Charge(ctx, adapter.ChargeRequest{
			PaymentID:      paymentID,
			CustomerID:     req.CustomerID,
			TokenReference: req.TokenReference,
			Amount:         req.Amount,
			Currency:       req.Currency,
		})
		if err != nil || !chgResp.Success {
			failReason := "CHARGE_DECLINED"
			if chgResp != nil && chgResp.FailureReason != "" {
				failReason = chgResp.FailureReason
			}
			_ = statemachine.Transition(ctx, s.db, paymentID, model.StatusFailed, failReason, &statemachine.TransitionOptions{
				FailureReason: &failReason,
			})
			p, _ := s.GetPayment(ctx, paymentID)
			return &InitiatePaymentResponse{Payment: p}, nil
		}

		// Direct 1-step capture
		err = statemachine.Transition(ctx, s.db, paymentID, model.StatusPending, "UILESS_INITIATED", nil)
		if err == nil {
			err = statemachine.Transition(ctx, s.db, paymentID, model.StatusCaptured, "UILESS_TOKEN_CAPTURE", &statemachine.TransitionOptions{
				ProviderReference: &chgResp.ProviderReference,
			})
		}
		p, _ := s.GetPayment(ctx, paymentID)
		return &InitiatePaymentResponse{Payment: p}, nil
	}

	// Standard UI flow: create hosted PGW session
	sessResp, err := adp.CreateSession(ctx, adapter.CreateSessionRequest{
		PaymentID:   paymentID,
		CustomerID:  req.CustomerID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		ReturnURL:   req.ReturnURL,
		CallbackURL: req.CallbackURL,
	})
	if err != nil {
		failReason := fmt.Sprintf("SESSION_CREATE_FAILED: %v", err)
		_ = statemachine.Transition(ctx, s.db, paymentID, model.StatusFailed, failReason, &statemachine.TransitionOptions{
			FailureReason: &failReason,
		})
		return nil, fmt.Errorf("failed to create provider session: %w", err)
	}

	// Transition CREATED -> PENDING
	err = statemachine.Transition(ctx, s.db, paymentID, model.StatusPending, "PGW_SESSION_CREATED", &statemachine.TransitionOptions{
		ProviderReference: &sessResp.ProviderReference,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to transition to PENDING: %w", err)
	}

	updatedPayment, err := s.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}

	return &InitiatePaymentResponse{
		Payment:     updatedPayment,
		RedirectURL: sessResp.RedirectURL,
		SessionID:   sessResp.SessionID,
	}, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, id string) (*model.Payment, error) {
	query := `
		SELECT id, idempotency_key, customer_id, amount, currency, method, provider, status, failure_reason, provider_reference, metadata, created_at, updated_at 
		FROM payments 
		WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var p model.Payment
	var statusStr string
	var failReason, provRef sql.NullString
	var metaBytes []byte

	if err := row.Scan(&p.ID, &p.IdempotencyKey, &p.CustomerID, &p.Amount, &p.Currency, &p.Method, &p.Provider, &statusStr, &failReason, &provRef, &metaBytes, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, statemachine.ErrPaymentNotFound
		}
		return nil, err
	}

	p.Status = model.PaymentStatus(statusStr)
	if failReason.Valid {
		p.FailureReason = &failReason.String
	}
	if provRef.Valid {
		p.ProviderReference = &provRef.String
	}
	p.Metadata = metaBytes

	return &p, nil
}

func (s *PaymentService) GetPaymentByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	query := `
		SELECT id, idempotency_key, customer_id, amount, currency, method, provider, status, failure_reason, provider_reference, metadata, created_at, updated_at 
		FROM payments 
		WHERE idempotency_key = ?`
	row := s.db.QueryRowContext(ctx, query, key)

	var p model.Payment
	var statusStr string
	var failReason, provRef sql.NullString
	var metaBytes []byte

	if err := row.Scan(&p.ID, &p.IdempotencyKey, &p.CustomerID, &p.Amount, &p.Currency, &p.Method, &p.Provider, &statusStr, &failReason, &provRef, &metaBytes, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	p.Status = model.PaymentStatus(statusStr)
	if failReason.Valid {
		p.FailureReason = &failReason.String
	}
	if provRef.Valid {
		p.ProviderReference = &provRef.String
	}
	p.Metadata = metaBytes

	return &p, nil
}

func (s *PaymentService) GetPaymentEvents(ctx context.Context, paymentID string) ([]model.PaymentEvent, error) {
	query := `
		SELECT id, payment_id, from_status, to_status, reason, created_at 
		FROM payment_events 
		WHERE payment_id = ? 
		ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []model.PaymentEvent
	for rows.Next() {
		var ev model.PaymentEvent
		if err := rows.Scan(&ev.ID, &ev.PaymentID, &ev.FromStatus, &ev.ToStatus, &ev.Reason, &ev.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *PaymentService) GetLedgerEntries(ctx context.Context, paymentID string) ([]model.LedgerEntry, error) {
	query := `
		SELECT id, payment_id, entry_type, account, amount, currency, reason, created_at 
		FROM ledger_entries 
		WHERE payment_id = ? 
		ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.LedgerEntry
	for rows.Next() {
		var entry model.LedgerEntry
		var entryTypeStr string
		if err := rows.Scan(&entry.ID, &entry.PaymentID, &entryTypeStr, &entry.Account, &entry.Amount, &entry.Currency, &entry.Reason, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.EntryType = model.LedgerEntryType(entryTypeStr)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ProcessWebhook handles deduplicated webhook notifications and maps them to state transitions.
func (s *PaymentService) ProcessWebhook(ctx context.Context, provider, providerEventID string, rawPayload json.RawMessage, targetPaymentID, eventType, reason, providerRef string) error {
	now := time.Now().UTC()
	webhookID := model.NewULID()

	// 1. Deduplicate by provider + provider_event_id
	insertWebhook := `
		INSERT INTO webhook_events (id, provider, provider_event_id, payload, received_at) 
		VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, insertWebhook, webhookID, provider, providerEventID, rawPayload, now)
	if err != nil {
		if isDuplicateEntryError(err) {
			// Already received and processed
			return ErrDuplicateWebhook
		}
		return fmt.Errorf("failed to record webhook event: %w", err)
	}

	// 2. Map provider event to state machine transition
	var targetStatus model.PaymentStatus
	switch strings.ToUpper(eventType) {
	case "PAYMENT.CAPTURED", "CHARGE_SUCCESS", "SUCCESS":
		targetStatus = model.StatusCaptured
	case "PAYMENT.FAILED", "CHARGE_FAILED", "DECLINED":
		targetStatus = model.StatusFailed
	case "PAYMENT.AUTHORIZED", "AUTH_SUCCESS":
		targetStatus = model.StatusAuthorized
	case "PAYMENT.EXPIRED", "EXPIRED":
		targetStatus = model.StatusExpired
	default:
		return fmt.Errorf("unknown webhook event type: %s", eventType)
	}

	opts := &statemachine.TransitionOptions{}
	if providerRef != "" {
		opts.ProviderReference = &providerRef
	}
	if reason != "" {
		opts.FailureReason = &reason
	}

	if err := statemachine.Transition(ctx, s.db, targetPaymentID, targetStatus, reason, opts); err != nil {
		return fmt.Errorf("failed to execute webhook transition: %w", err)
	}

	// 3. Mark processed
	updateQuery := `UPDATE webhook_events SET processed_at = ? WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, updateQuery, now, webhookID)

	return nil
}

// RefundPayment issues a full or partial refund for a CAPTURED payment.
func (s *PaymentService) RefundPayment(ctx context.Context, paymentID, idempotencyKey string, amount int64, reason string) (*model.Refund, error) {
	p, err := s.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p.Status != model.StatusCaptured {
		return nil, fmt.Errorf("cannot refund payment in status %s (must be CAPTURED)", p.Status)
	}

	now := time.Now().UTC()
	refundID := model.NewULID()

	if amount <= 0 || amount > p.Amount {
		amount = p.Amount // default full refund
	}

	// Call adapter
	adp, err := s.adapterRegistry.Get(p.Provider)
	if err == nil {
		provRef := ""
		if p.ProviderReference != nil {
			provRef = *p.ProviderReference
		}
		_, _ = adp.Refund(ctx, adapter.RefundRequest{
			PaymentID:         paymentID,
			ProviderReference: provRef,
			Amount:            amount,
			Reason:            reason,
		})
	}

	// Record in refunds table
	refundQuery := `
		INSERT INTO refunds (id, payment_id, idempotency_key, amount, currency, status, reason, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, refundQuery, refundID, paymentID, idempotencyKey, amount, p.Currency, string(model.RefundStatusSucceeded), reason, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to record refund: %w", err)
	}

	// Transition payment to REFUNDED (reverses ledger and queues outbox)
	err = statemachine.Transition(ctx, s.db, paymentID, model.StatusRefunded, reason, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to transition to REFUNDED: %w", err)
	}

	return &model.Refund{
		ID:             refundID,
		PaymentID:      paymentID,
		IdempotencyKey: idempotencyKey,
		Amount:         amount,
		Currency:       p.Currency,
		Status:         model.RefundStatusSucceeded,
		Reason:         reason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func isDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Error 1062") || strings.Contains(err.Error(), "Duplicate entry")
}
