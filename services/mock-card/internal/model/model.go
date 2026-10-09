package model

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

type CardSession struct {
	ID               string    `json:"id"`
	PaymentID        string    `json:"payment_id"`
	Amount           int64     `json:"amount"`
	Currency         string    `json:"currency"`
	Status           string    `json:"status"` // PENDING, SUCCESS, FAILED
	CardPANMasked    *string   `json:"card_pan_masked,omitempty"`
	CardBrand        *string   `json:"card_brand,omitempty"`
	TokenID          *string   `json:"token_id,omitempty"`
	CallbackURL      string    `json:"callback_url"`
	ReturnURL        string    `json:"return_url"`
	ScenarioOverride *string   `json:"scenario_override,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CardToken struct {
	ID             string    `json:"id"`
	CustomerID     string    `json:"customer_id"`
	CardBrand      string    `json:"card_brand"`
	CardLast4      string    `json:"card_last4"`
	ExpiryMonth    int       `json:"expiry_month"`
	ExpiryYear     int       `json:"expiry_year"`
	TokenReference string    `json:"token_reference"`
	Status         string    `json:"status"` // ACTIVE, REVOKED
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
