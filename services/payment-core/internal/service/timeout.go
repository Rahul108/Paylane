package service

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"payment-core/internal/adapter"
	"payment-core/internal/model"
	"payment-core/internal/statemachine"
)

type SweepResult struct {
	TotalChecked         int `json:"total_checked"`
	TransitionedCaptured int `json:"transitioned_captured"`
	TransitionedFailed   int `json:"transitioned_failed"`
	TransitionedExpired  int `json:"transitioned_expired"`
}

type TimeoutService struct {
	db              *sql.DB
	adapterRegistry *adapter.Registry
	logger          *slog.Logger
	stopCh          chan struct{}
}

func NewTimeoutService(db *sql.DB, adapterRegistry *adapter.Registry, logger *slog.Logger) *TimeoutService {
	return &TimeoutService{
		db:              db,
		adapterRegistry: adapterRegistry,
		logger:          logger,
		stopCh:          make(chan struct{}),
	}
}

// Start launches the background timeout sweep worker
func (t *TimeoutService) Start(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.stopCh:
				return
			case <-ticker.C:
				_, _ = t.Sweep(ctx, maxAge)
			}
		}
	}()
}

func (t *TimeoutService) Stop() {
	close(t.stopCh)
}

// Sweep finds payments stuck in PENDING or AUTHORIZED older than maxAge, verifies them, and transitions or expires them.
func (t *TimeoutService) Sweep(ctx context.Context, maxAge time.Duration) (*SweepResult, error) {
	cutoff := time.Now().UTC().Add(-maxAge)
	if maxAge <= 0 {
		cutoff = time.Now().UTC().Add(5 * time.Second)
	}

	query := `
		SELECT id, provider, provider_reference, status, created_at 
		FROM payments 
		WHERE status IN ('PENDING', 'AUTHORIZED') AND created_at <= ? 
		ORDER BY created_at ASC 
		LIMIT 50`

	rows, err := t.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		t.logger.Error("timeout sweep query failed", "err", err)
		return nil, err
	}
	defer rows.Close()

	type stuckPayment struct {
		id          string
		provider    string
		providerRef sql.NullString
		status      string
		createdAt   time.Time
	}

	var payments []stuckPayment
	for rows.Next() {
		var p stuckPayment
		if err := rows.Scan(&p.id, &p.provider, &p.providerRef, &p.status, &p.createdAt); err == nil {
			payments = append(payments, p)
		}
	}

	result := &SweepResult{
		TotalChecked: len(payments),
	}

	for _, p := range payments {
		provRef := ""
		if p.providerRef.Valid {
			provRef = p.providerRef.String
		}

		adp, err := t.adapterRegistry.Get(p.provider)
		if err != nil {
			t.logger.Warn("no adapter found for stuck payment", "payment_id", p.id, "provider", p.provider)
			continue
		}

		vResp, vErr := adp.Verify(ctx, adapter.VerifyRequest{
			PaymentID:         p.id,
			ProviderReference: provRef,
		})

		if vErr != nil {
			t.logger.Warn("verify check failed for stuck payment", "payment_id", p.id, "err", vErr)
			continue
		}

		switch vResp.Status {
		case "CAPTURED":
			t.logger.Info("timeout sweep: provider verified CAPTURED", "payment_id", p.id)
			opts := &statemachine.TransitionOptions{}
			if vResp.ProviderReference != "" {
				opts.ProviderReference = &vResp.ProviderReference
			}
			if err := statemachine.Transition(ctx, t.db, p.id, model.StatusCaptured, "TIMEOUT_SWEEP_VERIFIED_CAPTURED", opts); err == nil {
				result.TransitionedCaptured++
			}

		case "FAILED":
			t.logger.Info("timeout sweep: provider verified FAILED", "payment_id", p.id)
			failReason := "TIMEOUT_SWEEP_VERIFIED_FAILED"
			if err := statemachine.Transition(ctx, t.db, p.id, model.StatusFailed, failReason, &statemachine.TransitionOptions{
				FailureReason: &failReason,
			}); err == nil {
				result.TransitionedFailed++
			}

		default:
			// Still PENDING or unknown: payment has expired past the timeout limit!
			t.logger.Info("timeout sweep: payment expired past threshold", "payment_id", p.id)
			failReason := "TIMEOUT_EXPIRED"
			if err := statemachine.Transition(ctx, t.db, p.id, model.StatusExpired, failReason, &statemachine.TransitionOptions{
				FailureReason: &failReason,
			}); err == nil {
				result.TransitionedExpired++
			}
		}
	}

	if result.TotalChecked > 0 {
		t.logger.Info("timeout sweep completed",
			"checked", result.TotalChecked,
			"captured", result.TransitionedCaptured,
			"failed", result.TransitionedFailed,
			"expired", result.TransitionedExpired,
		)
	}

	return result, nil
}
