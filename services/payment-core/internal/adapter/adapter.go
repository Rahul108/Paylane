package adapter

import (
	"context"
)

type CreateSessionRequest struct {
	PaymentID   string `json:"payment_id"`
	CustomerID  string `json:"customer_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	ReturnURL   string `json:"return_url"`
	CallbackURL string `json:"callback_url"`
}

type CreateSessionResponse struct {
	SessionID         string `json:"session_id"`
	RedirectURL       string `json:"redirect_url"`
	ProviderReference string `json:"provider_reference"`
}

type ChargeRequest struct {
	PaymentID      string `json:"payment_id"`
	CustomerID     string `json:"customer_id"`
	TokenReference string `json:"token_reference"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
}

type ChargeResponse struct {
	Success           bool   `json:"success"`
	ProviderReference string `json:"provider_reference"`
	FailureReason     string `json:"failure_reason,omitempty"`
}

type VerifyRequest struct {
	PaymentID         string `json:"payment_id"`
	ProviderReference string `json:"provider_reference"`
}

type VerifyResponse struct {
	Status            string `json:"status"` // CAPTURED, PENDING, FAILED
	ProviderReference string `json:"provider_reference"`
	Amount            int64  `json:"amount"`
}

type RefundRequest struct {
	PaymentID         string `json:"payment_id"`
	ProviderReference string `json:"provider_reference"`
	Amount            int64  `json:"amount"`
	Reason            string `json:"reason"`
}

type RefundResponse struct {
	RefundID          string `json:"refund_id"`
	ProviderReference string `json:"provider_reference"`
	Status            string `json:"status"`
}

type BindRequest struct {
	CustomerID  string `json:"customer_id"`
	ReturnURL   string `json:"return_url"`
	CallbackURL string `json:"callback_url"`
}

type BindResponse struct {
	SessionID   string `json:"session_id"`
	RedirectURL string `json:"redirect_url"`
}

type UnbindRequest struct {
	CustomerID     string `json:"customer_id"`
	TokenReference string `json:"token_reference"`
}

type UnbindResponse struct {
	Success bool `json:"success"`
}

type SettlementRecord struct {
	SessionID   string `json:"session_id"`
	PaymentID   string `json:"payment_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Status      string `json:"status"` // SUCCESS
	CreatedAt   string `json:"created_at"`
}

type Adapter interface {
	Name() string
	CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error)
	Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error)
	Verify(ctx context.Context, req VerifyRequest) (*VerifyResponse, error)
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
	Bind(ctx context.Context, req BindRequest) (*BindResponse, error)
	Unbind(ctx context.Context, req UnbindRequest) (*UnbindResponse, error)
	GetSettlementReport(ctx context.Context) ([]SettlementRecord, error)
}
