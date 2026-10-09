package adapter

import (
	"context"
	"fmt"
	"payment-core/internal/model"
)

type MFSAdapter struct {
	baseURL string
}

func NewMFSAdapter(baseURL string) *MFSAdapter {
	if baseURL == "" {
		baseURL = "http://mock-mfs:5011"
	}
	return &MFSAdapter{baseURL: baseURL}
}

func (m *MFSAdapter) Name() string {
	return "mock-mfs"
}

func (m *MFSAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error) {
	sessionID := model.NewULID()
	providerRef := fmt.Sprintf("MFS_%s", sessionID)
	redirectURL := fmt.Sprintf("%s/checkout?session_id=%s&payment_id=%s&amount=%d", m.baseURL, sessionID, req.PaymentID, req.Amount)

	return &CreateSessionResponse{
		SessionID:         sessionID,
		RedirectURL:       redirectURL,
		ProviderReference: providerRef,
	}, nil
}

func (m *MFSAdapter) Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	// Directly charge bound token
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
