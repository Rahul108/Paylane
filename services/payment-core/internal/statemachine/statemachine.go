package statemachine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"payment-core/internal/model"
)

var (
	ErrPaymentNotFound    = errors.New("payment not found")
	ErrIllegalTransition  = errors.New("illegal payment status transition")
	ErrTerminalState      = errors.New("payment is in a terminal state")
)

// AllowedTransitions maps source status to valid target statuses.
var AllowedTransitions = map[model.PaymentStatus][]model.PaymentStatus{
	model.StatusCreated: {
		model.StatusPending,
	},
	model.StatusPending: {
		model.StatusAuthorized,
		model.StatusCaptured, // 1-step capture
		model.StatusFailed,
	},
	model.StatusAuthorized: {
		model.StatusCaptured,
		model.StatusFailed,
		model.StatusExpired,
	},
	model.StatusCaptured: {
		model.StatusRefunded,
	},
	model.StatusFailed:   {}, // terminal
	model.StatusExpired:  {}, // terminal
	model.StatusRefunded: {}, // terminal
}

func IsValidTransition(from, to model.PaymentStatus) bool {
	allowed, exists := AllowedTransitions[from]
	if !exists {
		return false
	}
	for _, target := range allowed {
		if target == to {
			return true
		}
	}
	return false
}

type TransitionOptions struct {
	ProviderReference *string
	FailureReason     *string
	CustomMetadata    map[string]any
}

// Transition executes a state machine transition inside an atomic database transaction.
// It enforces row locking, idempotency (quiet return on identical state), validity checks,
// updates payment status, records payment_events, writes double-entry ledger entries,
// and appends an outbox event.
func Transition(ctx context.Context, db *sql.DB, paymentID string, to model.PaymentStatus, reason string, opts *TransitionOptions) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := TransitionTx(ctx, tx, paymentID, to, reason, opts); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit tx: %w", err)
	}

	return nil
}

// TransitionTx executes the state transition within an existing transaction.
func TransitionTx(ctx context.Context, tx *sql.Tx, paymentID string, to model.PaymentStatus, reason string, opts *TransitionOptions) error {
	// 1. Lock payment row by primary key
	var currentStatusStr string
	var amount int64
	var currency, customerID, provider, method string
	var currentProviderRef sql.NullString

	query := `
		SELECT status, amount, currency, customer_id, provider, method, provider_reference 
		FROM payments 
		WHERE id = ? 
		FOR UPDATE`
	row := tx.QueryRowContext(ctx, query, paymentID)
	if err := row.Scan(&currentStatusStr, &amount, &currency, &customerID, &provider, &method, &currentProviderRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		return fmt.Errorf("failed to lock payment row: %w", err)
	}

	currentStatus := model.PaymentStatus(currentStatusStr)

	// 2. Idempotency check: if already in the target state, return quietly
	if currentStatus == to {
		return nil
	}

	// 3. Validate transition legality
	if currentStatus.IsTerminal() {
		return fmt.Errorf("%w: cannot transition from terminal status %s to %s", ErrTerminalState, currentStatus, to)
	}

	if !IsValidTransition(currentStatus, to) {
		return fmt.Errorf("%w: invalid transition from %s to %s", ErrIllegalTransition, currentStatus, to)
	}

	now := time.Now().UTC()

	// 4. Update payments table
	providerRef := currentProviderRef.String
	if opts != nil && opts.ProviderReference != nil && *opts.ProviderReference != "" {
		providerRef = *opts.ProviderReference
	}

	var failureReason sql.NullString
	if opts != nil && opts.FailureReason != nil && *opts.FailureReason != "" {
		failureReason = sql.NullString{String: *opts.FailureReason, Valid: true}
	}

	updateQuery := `
		UPDATE payments 
		SET status = ?, provider_reference = ?, failure_reason = ?, updated_at = ? 
		WHERE id = ?`
	if _, err := tx.ExecContext(ctx, updateQuery, string(to), providerRef, failureReason, now, paymentID); err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	// 5. Append payment_events audit record
	eventID := model.NewULID()
	eventQuery := `
		INSERT INTO payment_events (id, payment_id, from_status, to_status, reason, created_at) 
		VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, eventQuery, eventID, paymentID, string(currentStatus), string(to), reason, now); err != nil {
		return fmt.Errorf("failed to record payment event: %w", err)
	}

	// 6. Write Double-Entry Ledger entries
	if to == model.StatusCaptured {
		// Debit provider clearing / customer account, Credit merchant settlement account
		debitID := model.NewULID()
		creditID := model.NewULID()
		ledgerQuery := `
			INSERT INTO ledger_entries (id, payment_id, entry_type, account, amount, currency, reason, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		
		// Debit
		debitAccount := fmt.Sprintf("clearing_%s", provider)
		if _, err := tx.ExecContext(ctx, ledgerQuery, debitID, paymentID, string(model.LedgerDebit), debitAccount, amount, currency, "PAYMENT_CAPTURE", now); err != nil {
			return fmt.Errorf("failed to write debit ledger entry: %w", err)
		}

		// Credit
		creditAccount := "merchant_settlement"
		if _, err := tx.ExecContext(ctx, ledgerQuery, creditID, paymentID, string(model.LedgerCredit), creditAccount, amount, currency, "PAYMENT_CAPTURE", now); err != nil {
			return fmt.Errorf("failed to write credit ledger entry: %w", err)
		}
	} else if to == model.StatusRefunded {
		// Reversing entries: Debit merchant settlement, Credit customer / provider clearing
		debitID := model.NewULID()
		creditID := model.NewULID()
		ledgerQuery := `
			INSERT INTO ledger_entries (id, payment_id, entry_type, account, amount, currency, reason, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		
		creditAccount := fmt.Sprintf("clearing_%s", provider)
		debitAccount := "merchant_settlement"

		if _, err := tx.ExecContext(ctx, ledgerQuery, debitID, paymentID, string(model.LedgerDebit), debitAccount, amount, currency, "PAYMENT_REFUND", now); err != nil {
			return fmt.Errorf("failed to write refund debit ledger entry: %w", err)
		}
		if _, err := tx.ExecContext(ctx, ledgerQuery, creditID, paymentID, string(model.LedgerCredit), creditAccount, amount, currency, "PAYMENT_REFUND", now); err != nil {
			return fmt.Errorf("failed to write refund credit ledger entry: %w", err)
		}
	}

	// 7. Append Transactional Outbox Event
	outboxEventType := fmt.Sprintf("payment.%s", strings.ToLower(string(to)))
	outboxPayload, err := json.Marshal(map[string]any{
		"event_type":         outboxEventType,
		"payment_id":         paymentID,
		"customer_id":        customerID,
		"amount":             amount,
		"currency":           currency,
		"status":             string(to),
		"from_status":        string(currentStatus),
		"reason":             reason,
		"provider":           provider,
		"provider_reference": providerRef,
		"updated_at":         now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal outbox event: %w", err)
	}

	outboxID := model.NewULID()
	outboxQuery := `
		INSERT INTO outbox (id, event_type, aggregate_id, payload, status, retry_count, next_retry_at, created_at, updated_at) 
		VALUES (?, ?, ?, ?, 'PENDING', 0, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, outboxQuery, outboxID, outboxEventType, paymentID, outboxPayload, now, now, now); err != nil {
		return fmt.Errorf("failed to write outbox event: %w", err)
	}

	return nil
}
