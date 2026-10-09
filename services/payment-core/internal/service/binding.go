package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"payment-core/internal/adapter"
	"payment-core/internal/model"
)

var (
	ErrBindingNotFound = errors.New("binding not found")
)

type InitiateBindingRequest struct {
	CustomerID  string `json:"customer_id"`
	Method      string `json:"method"`   // mfs, card
	Provider    string `json:"provider"` // mock-mfs, mock-card
	ReturnURL   string `json:"return_url,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
}

type InitiateBindingResponse struct {
	SessionID   string `json:"session_id"`
	RedirectURL string `json:"redirect_url"`
}

type ConfirmBindingRequest struct {
	CustomerID       string  `json:"customer_id"`
	Method           string  `json:"method"`
	Provider         string  `json:"provider"`
	TokenReference   string  `json:"token_reference"`
	MaskedIdentifier string  `json:"masked_identifier"`
	CardBrand        *string `json:"card_brand,omitempty"`
	CardLast4        *string `json:"card_last4,omitempty"`
	ExpiryMonth      *int    `json:"expiry_month,omitempty"`
	ExpiryYear       *int    `json:"expiry_year,omitempty"`
}

func (s *PaymentService) InitiateBinding(ctx context.Context, req InitiateBindingRequest) (*InitiateBindingResponse, error) {
	if req.CustomerID == "" {
		return nil, fmt.Errorf("%w: customer_id is required", ErrInvalidRequest)
	}
	if req.Provider == "" {
		if req.Method == "mfs" {
			req.Provider = "mock-mfs"
		} else {
			req.Provider = "mock-card"
		}
	}

	adp, err := s.adapterRegistry.Get(req.Provider)
	if err != nil {
		return nil, err
	}

	resp, err := adp.Bind(ctx, adapter.BindRequest{
		CustomerID:  req.CustomerID,
		ReturnURL:   req.ReturnURL,
		CallbackURL: req.CallbackURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initiate binding with provider: %w", err)
	}

	return &InitiateBindingResponse{
		SessionID:   resp.SessionID,
		RedirectURL: resp.RedirectURL,
	}, nil
}

func (s *PaymentService) ConfirmBinding(ctx context.Context, req ConfirmBindingRequest) (*model.Binding, error) {
	now := time.Now().UTC()
	bindingID := model.NewULID()

	query := `
		INSERT INTO bindings (id, customer_id, method, provider, token_reference, masked_identifier, card_brand, card_last4, expiry_month, expiry_year, status, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'BOUND', ?, ?)`

	_, err := s.db.ExecContext(ctx, query, bindingID, req.CustomerID, req.Method, req.Provider, req.TokenReference, req.MaskedIdentifier, req.CardBrand, req.CardLast4, req.ExpiryMonth, req.ExpiryYear, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to record binding: %w", err)
	}

	// Append outbox event: binding.created
	outboxID := model.NewULID()
	payload, _ := json.Marshal(map[string]any{
		"binding_id":        bindingID,
		"customer_id":       req.CustomerID,
		"method":            req.Method,
		"provider":          req.Provider,
		"token_reference":   req.TokenReference,
		"masked_identifier": req.MaskedIdentifier,
		"status":            "BOUND",
		"timestamp":         now.Format(time.RFC3339Nano),
	})
	outboxQuery := `
		INSERT INTO outbox (id, event_type, aggregate_id, payload, status, retry_count, next_retry_at, created_at, updated_at) 
		VALUES (?, 'binding.created', ?, ?, 'PENDING', 0, ?, ?, ?)`
	_, _ = s.db.ExecContext(ctx, outboxQuery, outboxID, bindingID, payload, now, now, now)

	return &model.Binding{
		ID:               bindingID,
		CustomerID:       req.CustomerID,
		Method:           req.Method,
		Provider:         req.Provider,
		TokenReference:   req.TokenReference,
		MaskedIdentifier: req.MaskedIdentifier,
		CardBrand:        req.CardBrand,
		CardLast4:        req.CardLast4,
		ExpiryMonth:      req.ExpiryMonth,
		ExpiryYear:       req.ExpiryYear,
		Status:           model.BindingStatusBound,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *PaymentService) GetCustomerBindings(ctx context.Context, customerID string) ([]model.Binding, error) {
	query := `
		SELECT id, customer_id, method, provider, token_reference, masked_identifier, card_brand, card_last4, expiry_month, expiry_year, status, created_at, updated_at 
		FROM bindings 
		WHERE customer_id = ? AND status = 'BOUND' 
		ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []model.Binding
	for rows.Next() {
		var b model.Binding
		var statusStr string
		var brand, last4 sql.NullString
		var expM, expY sql.NullInt32
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.Method, &b.Provider, &b.TokenReference, &b.MaskedIdentifier, &brand, &last4, &expM, &expY, &statusStr, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		b.Status = model.BindingStatus(statusStr)
		if brand.Valid {
			b.CardBrand = &brand.String
		}
		if last4.Valid {
			b.CardLast4 = &last4.String
		}
		if expM.Valid {
			m := int(expM.Int32)
			b.ExpiryMonth = &m
		}
		if expY.Valid {
			y := int(expY.Int32)
			b.ExpiryYear = &y
		}
		bindings = append(bindings, b)
	}
	return bindings, rows.Err()
}

func (s *PaymentService) Unbind(ctx context.Context, bindingID string) error {
	var customerID, prov, tokenRef string
	query := `SELECT customer_id, provider, token_reference FROM bindings WHERE id = ? AND status = 'BOUND'`
	if err := s.db.QueryRowContext(ctx, query, bindingID).Scan(&customerID, &prov, &tokenRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBindingNotFound
		}
		return err
	}

	// Call provider to revoke
	adp, err := s.adapterRegistry.Get(prov)
	if err == nil {
		_, _ = adp.Unbind(ctx, adapter.UnbindRequest{
			CustomerID:     customerID,
			TokenReference: tokenRef,
		})
	}

	now := time.Now().UTC()
	// Soft update to UNBOUND
	updateQuery := `UPDATE bindings SET status = 'UNBOUND', updated_at = ? WHERE customer_id = ? AND token_reference = ?`
	if _, err := s.db.ExecContext(ctx, updateQuery, now, customerID, tokenRef); err != nil {
		return fmt.Errorf("failed to update binding: %w", err)
	}

	// Append outbox event: binding.removed
	outboxID := model.NewULID()
	payload, _ := json.Marshal(map[string]any{
		"binding_id":      bindingID,
		"customer_id":     customerID,
		"token_reference": tokenRef,
		"status":          "UNBOUND",
		"timestamp":       now.Format(time.RFC3339Nano),
	})
	outboxQuery := `
		INSERT INTO outbox (id, event_type, aggregate_id, payload, status, retry_count, next_retry_at, created_at, updated_at) 
		VALUES (?, 'binding.removed', ?, ?, 'PENDING', 0, ?, ?, ?)`
	_, _ = s.db.ExecContext(ctx, outboxQuery, outboxID, bindingID, payload, now, now, now)

	return nil
}
