package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mock-card/internal/model"
	"paylane-jwe"
)

var (
	ErrSessionNotFound = errors.New("card session not found")
)

type CreateSessionRequest struct {
	PaymentID   string `json:"payment_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	CallbackURL string `json:"callback_url"`
	ReturnURL   string `json:"return_url"`
	Scenario    string `json:"scenario,omitempty"`
}

type CreateSessionResponse struct {
	SessionID         string `json:"session_id"`
	RedirectURL       string `json:"redirect_url"`
	ProviderReference string `json:"provider_reference"`
}

type Service struct {
	db        *sql.DB
	jweClient *jwe.Client
	logger    *slog.Logger
	baseURL   string
}

func NewService(db *sql.DB, jweClient *jwe.Client, baseURL string, logger *slog.Logger) *Service {
	if baseURL == "" {
		baseURL = "http://localhost:5012"
	}
	return &Service{
		db:        db,
		jweClient: jweClient,
		baseURL:   baseURL,
		logger:    logger,
	}
}

func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error) {
	sessionID := model.NewULID()
	now := time.Now().UTC()

	if req.Currency == "" {
		req.Currency = "BDT"
	}
	if req.CallbackURL == "" {
		req.CallbackURL = "http://payment-core:4011/webhooks/mock-card"
	}
	if req.ReturnURL == "" {
		req.ReturnURL = "http://localhost:3010/return"
	}

	query := `
		INSERT INTO card_sessions (id, payment_id, amount, currency, status, callback_url, return_url, scenario_override, created_at, updated_at) 
		VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?, ?, ?)`

	var scen sql.NullString
	if req.Scenario != "" {
		scen = sql.NullString{String: req.Scenario, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, query, sessionID, req.PaymentID, req.Amount, req.Currency, req.CallbackURL, req.ReturnURL, scen, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	redirectURL := fmt.Sprintf("%s/checkout?session_id=%s", s.baseURL, sessionID)
	provRef := fmt.Sprintf("CRD_%s", sessionID)

	return &CreateSessionResponse{
		SessionID:         sessionID,
		RedirectURL:       redirectURL,
		ProviderReference: provRef,
	}, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (*model.CardSession, error) {
	query := `
		SELECT id, payment_id, amount, currency, status, card_pan_masked, card_brand, token_id, callback_url, return_url, scenario_override, created_at, updated_at 
		FROM card_sessions 
		WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var sess model.CardSession
	var pan, brand, token, scen sql.NullString
	if err := row.Scan(&sess.ID, &sess.PaymentID, &sess.Amount, &sess.Currency, &sess.Status, &pan, &brand, &token, &sess.CallbackURL, &sess.ReturnURL, &scen, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	if pan.Valid {
		sess.CardPANMasked = &pan.String
	}
	if brand.Valid {
		sess.CardBrand = &brand.String
	}
	if token.Valid {
		sess.TokenID = &token.String
	}
	if scen.Valid {
		sess.ScenarioOverride = &scen.String
	}

	return &sess, nil
}

func (s *Service) ConfirmPayment(ctx context.Context, sessionID, cardholder, pan, expiry, cvv, otp, scenario string) (string, error) {
	sess, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return "", err
	}

	if scenario == "" && sess.ScenarioOverride != nil {
		scenario = *sess.ScenarioOverride
	}
	if scenario == "" {
		scenario = "SUCCESS"
	}

	now := time.Now().UTC()
	var finalStatus = "SUCCESS"
	var eventType = "PAYMENT.CAPTURED"
	var reason = "CARD_AUTHORIZED_AND_CAPTURED"
	var brand = "Visa"
	var maskedPAN = "411111****1111"

	cleanPAN := strings.ReplaceAll(pan, " ", "")
	if len(cleanPAN) >= 4 {
		maskedPAN = fmt.Sprintf("%s****%s", cleanPAN[:4], cleanPAN[len(cleanPAN)-4:])
	}
	if strings.HasPrefix(cleanPAN, "5") {
		brand = "Mastercard"
	}

	// Test Card Rules
	switch cleanPAN {
	case "4000000000000002":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "INSUFFICIENT_FUNDS"
	case "4000000000000003":
		// 3DS OTP validation
		if otp != "123456" {
			finalStatus = "FAILED"
			eventType = "PAYMENT.FAILED"
			reason = "3DS_OTP_FAILED"
		}
	case "4000000000000004":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "GATEWAY_TIMEOUT"
	}

	// Scenario override
	switch strings.ToUpper(scenario) {
	case "INSUFFICIENT_FUNDS":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "INSUFFICIENT_FUNDS"
	case "DECLINED":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "DO_NOT_HONOR"
	case "USER_ABANDONS":
		updateQ := `UPDATE card_sessions SET status = 'FAILED', updated_at = ? WHERE id = ?`
		_, _ = s.db.ExecContext(ctx, updateQ, now, sessionID)
		return sess.ReturnURL + "?status=abandoned", nil
	case "RECONCILIATION_MISMATCH", "NO_CALLBACK":
		updateQ := `UPDATE card_sessions SET status = 'SUCCESS', card_pan_masked = ?, card_brand = ?, updated_at = ? WHERE id = ?`
		_, _ = s.db.ExecContext(ctx, updateQ, maskedPAN, brand, now, sessionID)
		delim := "?"
		if strings.Contains(sess.ReturnURL, "?") {
			delim = "&"
		}
		return fmt.Sprintf("%s%sstatus=success&payment_id=%s", sess.ReturnURL, delim, sess.PaymentID), nil
	case "AMOUNT_MISMATCH":
		updateQ := `UPDATE card_sessions SET status = 'SUCCESS', amount = amount + 5000, card_pan_masked = ?, card_brand = ?, updated_at = ? WHERE id = ?`
		_, _ = s.db.ExecContext(ctx, updateQ, maskedPAN, brand, now, sessionID)
		finalStatus = "SUCCESS"
		eventType = "PAYMENT.CAPTURED"
		reason = "CARD_AUTHORIZED_AND_CAPTURED"
	}

	// Update session
	updateQuery := `UPDATE card_sessions SET status = ?, card_pan_masked = ?, card_brand = ?, updated_at = ? WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, updateQuery, finalStatus, maskedPAN, brand, now, sessionID)

	// Send JWE Webhook callback to payment-core
	webhookPayload := map[string]any{
		"event_id":           fmt.Sprintf("wh_crd_%s", model.NewULID()),
		"event_type":         eventType,
		"payment_id":         sess.PaymentID,
		"provider_reference": fmt.Sprintf("CRD_%s", sessionID),
		"reason":             reason,
	}
	webhookBytes, _ := json.Marshal(webhookPayload)

	if s.jweClient != nil && sess.CallbackURL != "" {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			code, respBytes, err := s.jweClient.Post(bgCtx, "payment-core", sess.CallbackURL, webhookBytes, nil)
			if err != nil {
				s.logger.Error("failed to dispatch JWE webhook to payment-core", "err", err, "url", sess.CallbackURL)
			} else {
				s.logger.Info("dispatched JWE webhook to payment-core successfully", "status", code, "response", string(respBytes))
			}
		}()
	}

	delim := "?"
	if strings.Contains(sess.ReturnURL, "?") {
		delim = "&"
	}
	targetURL := fmt.Sprintf("%s%sstatus=%s&payment_id=%s", sess.ReturnURL, delim, strings.ToLower(finalStatus), sess.PaymentID)
	return targetURL, nil
}

func (s *Service) ChargeToken(ctx context.Context, tokenRef string, amount int64, currency string) (bool, string, error) {
	provRef := fmt.Sprintf("CRD_CHG_%s", model.NewULID())
	return true, provRef, nil
}

func (s *Service) Refund(ctx context.Context, paymentID, provRef string, amount int64) (string, error) {
	rfdID := fmt.Sprintf("CRD_RFD_%s", model.NewULID())
	return rfdID, nil
}

func (s *Service) GetSettlementReport(ctx context.Context) ([]map[string]any, error) {
	query := `
		SELECT id, payment_id, amount, currency, status, card_pan_masked, card_brand, created_at 
		FROM card_sessions 
		WHERE status = 'SUCCESS' 
		ORDER BY created_at DESC 
		LIMIT 100`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var report []map[string]any
	for rows.Next() {
		var id, payID, curr, status string
		var amount int64
		var pan, brand sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&id, &payID, &amount, &curr, &status, &pan, &brand, &createdAt); err != nil {
			continue
		}
		report = append(report, map[string]any{
			"session_id": id,
			"payment_id": payID,
			"amount":     amount,
			"currency":   curr,
			"status":     status,
			"card_pan":   pan.String,
			"brand":      brand.String,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}
	return report, nil
}
