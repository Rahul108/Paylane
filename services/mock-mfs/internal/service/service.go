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

	"mock-mfs/internal/model"
	"paylane-jwe"
)

var (
	ErrSessionNotFound = errors.New("mfs session not found")
	ErrInvalidPIN      = errors.New("invalid mfs pin")
	ErrInvalidOTP      = errors.New("invalid mfs otp")
	ErrInsufficientBal = errors.New("insufficient wallet balance")
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
		baseURL = "http://localhost:5011"
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
		req.CallbackURL = "http://payment-core:4011/webhooks/mock-mfs"
	}
	if req.ReturnURL == "" {
		req.ReturnURL = "http://localhost:3010/return"
	}

	query := `
		INSERT INTO mfs_sessions (id, payment_id, amount, currency, status, callback_url, return_url, scenario_override, created_at, updated_at) 
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
	provRef := fmt.Sprintf("MFS_%s", sessionID)

	return &CreateSessionResponse{
		SessionID:         sessionID,
		RedirectURL:       redirectURL,
		ProviderReference: provRef,
	}, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (*model.MFSSession, error) {
	query := `
		SELECT id, payment_id, amount, currency, status, msisdn, callback_url, return_url, scenario_override, created_at, updated_at 
		FROM mfs_sessions 
		WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var sess model.MFSSession
	var msisdn, scen sql.NullString
	if err := row.Scan(&sess.ID, &sess.PaymentID, &sess.Amount, &sess.Currency, &sess.Status, &msisdn, &sess.CallbackURL, &sess.ReturnURL, &scen, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	if msisdn.Valid {
		sess.MSISDN = &msisdn.String
	}
	if scen.Valid {
		sess.ScenarioOverride = &scen.String
	}

	return &sess, nil
}

func (s *Service) ConfirmPayment(ctx context.Context, sessionID, msisdn, pin, otp, scenario string) (string, error) {
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
	var reason = "MFS_PIN_OTP_VERIFIED"
	var returnURL = sess.ReturnURL

	switch strings.ToUpper(scenario) {
	case "INSUFFICIENT_FUNDS":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "INSUFFICIENT_WALLET_BALANCE"
	case "WRONG_PIN":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "INVALID_WALLET_PIN"
	case "WRONG_OTP":
		finalStatus = "FAILED"
		eventType = "PAYMENT.FAILED"
		reason = "INVALID_WALLET_OTP"
	case "USER_ABANDONS":
		// User aborted: do not dispatch webhook callback
		updateQ := `UPDATE mfs_sessions SET status = 'FAILED', updated_at = ? WHERE id = ?`
		_, _ = s.db.ExecContext(ctx, updateQ, now, sessionID)
		return sess.ReturnURL + "?status=abandoned", nil
	default:
		// Normal verification
		if pin != "1234" {
			finalStatus = "FAILED"
			eventType = "PAYMENT.FAILED"
			reason = "INVALID_WALLET_PIN"
		} else if otp != "" && otp != "123456" {
			finalStatus = "FAILED"
			eventType = "PAYMENT.FAILED"
			reason = "INVALID_WALLET_OTP"
		}
	}

	// Update session
	updateQuery := `UPDATE mfs_sessions SET status = ?, msisdn = ?, updated_at = ? WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, updateQuery, finalStatus, msisdn, now, sessionID)

	// Send webhook callback to payment-core over JWE
	webhookPayload := map[string]any{
		"event_id":           fmt.Sprintf("wh_mfs_%s", model.NewULID()),
		"event_type":         eventType,
		"payment_id":         sess.PaymentID,
		"provider_reference": fmt.Sprintf("MFS_%s", sessionID),
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
	if strings.Contains(returnURL, "?") {
		delim = "&"
	}
	targetURL := fmt.Sprintf("%s%sstatus=%s&payment_id=%s", returnURL, delim, strings.ToLower(finalStatus), sess.PaymentID)
	return targetURL, nil
}

func (s *Service) ChargeAgreement(ctx context.Context, agreementID string, amount int64, currency string) (bool, string, error) {
	provRef := fmt.Sprintf("MFS_AGR_%s", model.NewULID())
	return true, provRef, nil
}

func (s *Service) GetSettlementReport(ctx context.Context) ([]map[string]any, error) {
	query := `
		SELECT id, payment_id, amount, currency, status, msisdn, created_at 
		FROM mfs_sessions 
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
		var msisdn sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&id, &payID, &amount, &curr, &status, &msisdn, &createdAt); err != nil {
			continue
		}
		report = append(report, map[string]any{
			"session_id": id,
			"payment_id": payID,
			"amount":     amount,
			"currency":   curr,
			"status":     status,
			"msisdn":     msisdn.String,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}
	return report, nil
}
