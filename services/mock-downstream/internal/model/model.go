package model

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

type DownstreamCall struct {
	ID               string    `json:"id"`
	CallType         string    `json:"call_type"` // recharge, cashback, subscription
	IdempotencyKey   string    `json:"idempotency_key"`
	CustomerID       string    `json:"customer_id"`
	Amount           *int64    `json:"amount,omitempty"`
	Status           string    `json:"status"` // SUCCEEDED, FAILED
	ScenarioOverride *string   `json:"scenario_override,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func NewULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
