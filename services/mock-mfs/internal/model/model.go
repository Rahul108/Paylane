package model

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

type MFSSession struct {
	ID               string    `json:"id"`
	PaymentID        string    `json:"payment_id"`
	Amount           int64     `json:"amount"`
	Currency         string    `json:"currency"`
	Status           string    `json:"status"` // PENDING, SUCCESS, FAILED
	MSISDN           *string   `json:"msisdn,omitempty"`
	CallbackURL      string    `json:"callback_url"`
	ReturnURL        string    `json:"return_url"`
	ScenarioOverride *string   `json:"scenario_override,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type MFSAgreement struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	MSISDN     string    `json:"msisdn"`
	Status     string    `json:"status"` // ACTIVE, REVOKED
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NewULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
