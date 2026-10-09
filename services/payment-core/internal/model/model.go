package model

import (
	"crypto/rand"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"
)

type PaymentStatus string

const (
	StatusCreated    PaymentStatus = "CREATED"
	StatusPending    PaymentStatus = "PENDING"
	StatusAuthorized PaymentStatus = "AUTHORIZED"
	StatusCaptured   PaymentStatus = "CAPTURED"
	StatusFailed     PaymentStatus = "FAILED"
	StatusExpired    PaymentStatus = "EXPIRED"
	StatusRefunded   PaymentStatus = "REFUNDED"
)

func (s PaymentStatus) IsTerminal() bool {
	return s == StatusFailed || s == StatusExpired || s == StatusRefunded
}

type Payment struct {
	ID                string          `json:"id"`
	IdempotencyKey    string          `json:"idempotency_key"`
	CustomerID        string          `json:"customer_id"`
	Amount            int64           `json:"amount"` // in smallest currency subunit (e.g. Paisa)
	Currency          string          `json:"currency"`
	Method            string          `json:"method"`   // mfs, card
	Provider          string          `json:"provider"` // mock-mfs, mock-card
	Status            PaymentStatus   `json:"status"`
	FailureReason     *string         `json:"failure_reason,omitempty"`
	ProviderReference *string         `json:"provider_reference,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type PaymentEvent struct {
	ID         string    `json:"id"`
	PaymentID  string    `json:"payment_id"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

type LedgerEntryType string

const (
	LedgerDebit  LedgerEntryType = "DEBIT"
	LedgerCredit LedgerEntryType = "CREDIT"
)

type LedgerEntry struct {
	ID        string          `json:"id"`
	PaymentID string          `json:"payment_id"`
	EntryType LedgerEntryType `json:"entry_type"` // DEBIT, CREDIT
	Account   string          `json:"account"`
	Amount    int64           `json:"amount"`
	Currency  string          `json:"currency"`
	Reason    string          `json:"reason"`
	CreatedAt time.Time       `json:"created_at"`
}

type RefundStatus string

const (
	RefundStatusPending   RefundStatus = "PENDING"
	RefundStatusSucceeded RefundStatus = "SUCCEEDED"
	RefundStatusFailed    RefundStatus = "FAILED"
)

type Refund struct {
	ID             string       `json:"id"`
	PaymentID      string       `json:"payment_id"`
	IdempotencyKey string       `json:"idempotency_key"`
	Amount         int64        `json:"amount"`
	Currency       string       `json:"currency"`
	Status         RefundStatus `json:"status"`
	Reason         string       `json:"reason"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type BindingStatus string

const (
	BindingStatusBound   BindingStatus = "BOUND"
	BindingStatusUnbound BindingStatus = "UNBOUND"
)

type Binding struct {
	ID               string        `json:"id"`
	CustomerID       string        `json:"customer_id"`
	Method           string        `json:"method"`
	Provider         string        `json:"provider"`
	TokenReference   string        `json:"token_reference"`
	MaskedIdentifier string        `json:"masked_identifier"`
	CardBrand        *string       `json:"card_brand,omitempty"`
	CardLast4        *string       `json:"card_last4,omitempty"`
	ExpiryMonth      *int          `json:"expiry_month,omitempty"`
	ExpiryYear       *int          `json:"expiry_year,omitempty"`
	Status           BindingStatus `json:"status"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "PENDING"
	OutboxPublished OutboxStatus = "PUBLISHED"
	OutboxFailed    OutboxStatus = "FAILED"
)

type OutboxEvent struct {
	ID          string          `json:"id"`
	EventType   string          `json:"event_type"` // payment.captured, payment.failed, etc.
	AggregateID string          `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
	Status      OutboxStatus    `json:"status"`
	RetryCount  int             `json:"retry_count"`
	NextRetryAt time.Time       `json:"next_retry_at"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type WebhookEvent struct {
	ID              string          `json:"id"`
	Provider        string          `json:"provider"`
	ProviderEventID string          `json:"provider_event_id"`
	Payload         json.RawMessage `json:"payload"`
	ReceivedAt      time.Time       `json:"received_at"`
	ProcessedAt     *time.Time      `json:"processed_at,omitempty"`
}

// NewULID generates a canonical 26-character ULID string.
func NewULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
