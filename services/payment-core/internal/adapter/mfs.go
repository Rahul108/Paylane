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

type MFSAdapter struct {
	baseURL    string
	httpClient *http.Client
}

func NewMFSAdapter(baseURL string) *MFSAdapter {
	if baseURL == "" {
		baseURL = "http://mock-mfs:5011"
	}
	return &MFSAdapter{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *MFSAdapter) Name() string {
	return "mock-mfs"
}

func (m *MFSAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"payment_id":   req.PaymentID,
		"amount":       req.Amount,
		"currency":     req.Currency,
		"callback_url": req.CallbackURL,
		"return_url":   req.ReturnURL,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/sessions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		// Graceful fallback for offline / unit tests
		sessionID := model.NewULID()
		return &CreateSessionResponse{
			SessionID:         sessionID,
			RedirectURL:       fmt.Sprintf("%s/checkout?session_id=%s", m.baseURL, sessionID),
			ProviderReference: fmt.Sprintf("MFS_%s", sessionID),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mock-mfs returned status %d", resp.StatusCode)
	}

	var res CreateSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (m *MFSAdapter) Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	// Directly charge bound agreement token
	providerRef := fmt.Sprintf("MFS_CHG_%s", model.NewULID())
	return &ChargeResponse{
		Success:           true,
		ProviderReference: providerRef,
	}, nil
}

func (m *MFSAdapter) Verify(ctx context.Context, req VerifyRequest) (*VerifyResponse, error) {
	return &VerifyResponse{
		Status:            "CAPTURED",
		ProviderReference: req.ProviderReference,
	}, nil
}

func (m *MFSAdapter) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	refundID := fmt.Sprintf("MFS_RFD_%s", model.NewULID())
	return &RefundResponse{
		RefundID:          refundID,
		ProviderReference: req.ProviderReference,
		Status:            "SUCCEEDED",
	}, nil
}

func (m *MFSAdapter) Bind(ctx context.Context, req BindRequest) (*BindResponse, error) {
	sessionID := model.NewULID()
	redirectURL := fmt.Sprintf("%s/bind?session_id=%s&customer_id=%s", m.baseURL, sessionID, req.CustomerID)
	return &BindResponse{
		SessionID:   sessionID,
		RedirectURL: redirectURL,
	}, nil
}

func (m *MFSAdapter) Unbind(ctx context.Context, req UnbindRequest) (*UnbindResponse, error) {
	return &UnbindResponse{Success: true}, nil
}
