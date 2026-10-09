package model

import (
	"crypto/rand"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"
)

type JourneyStatus string

const (
	JourneyStatusStarted         JourneyStatus = "STARTED"
	JourneyStatusAwaitingPayment JourneyStatus = "AWAITING_PAYMENT"
	JourneyStatusRunningSteps    JourneyStatus = "RUNNING_STEPS"
	JourneyStatusCompleted       JourneyStatus = "COMPLETED"
	JourneyStatusFailed          JourneyStatus = "FAILED"
	JourneyStatusNeedsAttention  JourneyStatus = "NEEDS_ATTENTION"
)

type StepStatus string

const (
	StepStatusPending        StepStatus = "PENDING"
	StepStatusRunning        StepStatus = "RUNNING"
	StepStatusSucceeded      StepStatus = "SUCCEEDED"
	StepStatusFailed         StepStatus = "FAILED"
	StepStatusRetrying       StepStatus = "RETRYING"
	StepStatusNeedsAttention StepStatus = "NEEDS_ATTENTION"
)

type Journey struct {
	ID           string          `json:"id"`
	JourneyType  string          `json:"journey_type"`
	CustomerID   string          `json:"customer_id"`
	Status       JourneyStatus   `json:"status"`
	InputPayload json.RawMessage `json:"input_payload"`
	CurrentStep  *string         `json:"current_step,omitempty"`
	PaymentID    *string         `json:"payment_id,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Steps        []JourneyStep   `json:"steps,omitempty"`
}

type JourneyStep struct {
	ID                  string          `json:"id"`
	JourneyID           string          `json:"journey_id"`
	StepName            string          `json:"step_name"`
	StepOrder           int             `json:"step_order"`
	Status              StepStatus      `json:"status"`
	AttemptCount        int             `json:"attempt_count"`
	LastError           *string         `json:"last_error,omitempty"`
	DownstreamReference *string         `json:"downstream_reference,omitempty"`
	RequestSummary      json.RawMessage `json:"request_summary,omitempty"`
	ResponseSummary     json.RawMessage `json:"response_summary,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

func NewULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
