package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"payment-core/internal/model"
)

type CardAdapter struct {
	baseURL    string
	httpClient *http.Client
}

func NewCardAdapter(baseURL string) *CardAdapter {
	if baseURL == "" {
		baseURL = "http://mock-card:5012"
	}
	return &CardAdapter{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *CardAdapter) Name() string {
	return "mock-card"
}

func (c *CardAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"payment_id":   req.PaymentID,
		"amount":       req.Amount,
		"currency":     req.Currency,
		"callback_url": req.CallbackURL,
		"return_url":   req.ReturnURL,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sessions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// Fallback for offline / unit tests
		sessionID := model.NewULID()
		return &CreateSessionResponse{
			SessionID:         sessionID,
			RedirectURL:       fmt.Sprintf("%s/checkout?session_id=%s", c.baseURL, sessionID),
			ProviderReference: fmt.Sprintf("CRD_%s", sessionID),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mock-card returned status %d", resp.StatusCode)
	}

	var res CreateSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *CardAdapter) Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	providerRef := fmt.Sprintf("CRD_CHG_%s", model.NewULID())
	return &ChargeResponse{
		Success:           true,
		ProviderReference: providerRef,
	}, nil
}

func (c *CardAdapter) Verify(ctx context.Context, req VerifyRequest) (*VerifyResponse, error) {
	return &VerifyResponse{
		Status:            "CAPTURED",
		ProviderReference: req.ProviderReference,
	}, nil
}

func (c *CardAdapter) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	refundID := fmt.Sprintf("CRD_RFD_%s", model.NewULID())
	return &RefundResponse{
		RefundID:          refundID,
		ProviderReference: req.ProviderReference,
		Status:            "SUCCEEDED",
	}, nil
}

func (c *CardAdapter) Bind(ctx context.Context, req BindRequest) (*BindResponse, error) {
	sessionID := model.NewULID()
	redirectURL := fmt.Sprintf("%s/bind?session_id=%s&customer_id=%s", c.baseURL, sessionID, req.CustomerID)
	return &BindResponse{
		SessionID:   sessionID,
		RedirectURL: redirectURL,
	}, nil
}

func (c *CardAdapter) Unbind(ctx context.Context, req UnbindRequest) (*UnbindResponse, error) {
	return &UnbindResponse{Success: true}, nil
}
