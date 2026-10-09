package adapter

import (
	"context"
	"fmt"
	"payment-core/internal/model"
)

type CardAdapter struct {
	baseURL string
}

func NewCardAdapter(baseURL string) *CardAdapter {
	if baseURL == "" {
		baseURL = "http://mock-card:5012"
	}
	return &CardAdapter{baseURL: baseURL}
}

func (c *CardAdapter) Name() string {
	return "mock-card"
}

func (c *CardAdapter) CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error) {
	sessionID := model.NewULID()
	providerRef := fmt.Sprintf("CRD_%s", sessionID)
	redirectURL := fmt.Sprintf("%s/checkout?session_id=%s&payment_id=%s&amount=%d", c.baseURL, sessionID, req.PaymentID, req.Amount)

	return &CreateSessionResponse{
		SessionID:         sessionID,
		RedirectURL:       redirectURL,
		ProviderReference: providerRef,
	}, nil
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
