package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"payment-core/internal/adapter"
	"payment-core/internal/model"
)

type ReconciliationItem struct {
	ID                string          `json:"id"`
	PaymentID         *string         `json:"payment_id,omitempty"`
	Provider          string          `json:"provider"`
	ProviderReference *string         `json:"provider_reference,omitempty"`
	MismatchType      string          `json:"mismatch_type"`
	Details           json.RawMessage `json:"details"`
	ResolvedAt        *time.Time      `json:"resolved_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

type ReconciliationReport struct {
	TotalRecordsChecked int                  `json:"total_records_checked"`
	MismatchesFound     int                  `json:"mismatches_found"`
	Items               []ReconciliationItem `json:"items"`
}

type ReconciliationService struct {
	db              *sql.DB
	adapterRegistry *adapter.Registry
	logger          *slog.Logger
}

func NewReconciliationService(db *sql.DB, adapterRegistry *adapter.Registry, logger *slog.Logger) *ReconciliationService {
	return &ReconciliationService{
		db:              db,
		adapterRegistry: adapterRegistry,
		logger:          logger,
	}
}

// RunReconciliation pulls settlement reports from mock PGWs, compares with payments and ledger, and logs discrepancies.
func (r *ReconciliationService) RunReconciliation(ctx context.Context, targetProvider string) (*ReconciliationReport, error) {
	now := time.Now().UTC()
	report := &ReconciliationReport{}

	providers := []string{"mock-mfs", "mock-card"}
	if targetProvider != "" {
		providers = []string{targetProvider}
	}

	for _, prov := range providers {
		adp, err := r.adapterRegistry.Get(prov)
		if err != nil {
			r.logger.Warn("reconciliation: adapter not found", "provider", prov)
			continue
		}

		records, err := adp.GetSettlementReport(ctx)
		if err != nil {
			r.logger.Error("failed to get settlement report", "provider", prov, "err", err)
			continue
		}

		report.TotalRecordsChecked += len(records)

		for _, item := range records {
			r.reconcileRecord(ctx, prov, item, now, report)
		}
	}

	r.logger.Info("reconciliation run finished",
		"checked", report.TotalRecordsChecked,
		"mismatches", report.MismatchesFound,
	)

	return report, nil
}

func (r *ReconciliationService) reconcileRecord(ctx context.Context, provider string, item adapter.SettlementRecord, now time.Time, report *ReconciliationReport) {
	var paymentID, status, provRef string
	var amount int64

	pQuery := `SELECT id, status, amount, provider_reference FROM payments WHERE id = ?`
	err := r.db.QueryRowContext(ctx, pQuery, item.PaymentID).Scan(&paymentID, &status, &amount, &provRef)

	if err != nil {
		if err == sql.ErrNoRows {
			// Record exists at PGW but is missing entirely in core!
			r.recordMismatch(ctx, &item.PaymentID, provider, &item.SessionID, "MISSING_IN_CORE", map[string]any{
				"session_id": item.SessionID,
				"amount":     item.Amount,
				"currency":   item.Currency,
				"status":     item.Status,
				"note":       "Provider settlement record does not exist in payment-core",
			}, now, report)
			return
		}
		r.logger.Error("error querying payment for reconciliation", "payment_id", item.PaymentID, "err", err)
		return
	}

	// 1. Status Mismatch: PGW says SUCCESS, but core is not CAPTURED (or REFUNDED)
	if item.Status == "SUCCESS" && status != string(model.StatusCaptured) && status != string(model.StatusRefunded) {
		r.recordMismatch(ctx, &paymentID, provider, &item.SessionID, "STATUS_MISMATCH", map[string]any{
			"provider_status": item.Status,
			"core_status":     status,
			"amount":          amount,
			"note":            fmt.Sprintf("Provider recorded SUCCESS but payment-core is %s", status),
		}, now, report)
	}

	// 2. Amount Mismatch
	if item.Amount != amount {
		r.recordMismatch(ctx, &paymentID, provider, &item.SessionID, "AMOUNT_MISMATCH", map[string]any{
			"provider_amount": item.Amount,
			"core_amount":     amount,
			"note":            fmt.Sprintf("Provider amount %d != core amount %d", item.Amount, amount),
		}, now, report)
	}

	// 3. Ledger verification for CAPTURED payments
	if status == string(model.StatusCaptured) {
		var debitCount, creditCount int
		ledgerQ := `
			SELECT 
				COUNT(CASE WHEN entry_type = 'DEBIT' THEN 1 END),
				COUNT(CASE WHEN entry_type = 'CREDIT' THEN 1 END)
			FROM ledger_entries 
			WHERE payment_id = ?`
		_ = r.db.QueryRowContext(ctx, ledgerQ, paymentID).Scan(&debitCount, &creditCount)

		if debitCount == 0 || creditCount == 0 {
			r.recordMismatch(ctx, &paymentID, provider, &item.SessionID, "LEDGER_MISMATCH", map[string]any{
				"debit_entries":  debitCount,
				"credit_entries": creditCount,
				"note":           "Missing balanced double-entry ledger rows for captured payment",
			}, now, report)
		}
	}
}

func (r *ReconciliationService) recordMismatch(ctx context.Context, paymentID *string, provider string, provRef *string, mismatchType string, details map[string]any, now time.Time, report *ReconciliationReport) {
	// Deduplicate: avoid duplicate unresolved mismatch for the same payment and mismatch_type
	var existingID string
	checkQ := `
		SELECT id FROM reconciliation_items 
		WHERE provider = ? AND mismatch_type = ? AND resolved_at IS NULL AND (payment_id = ? OR (payment_id IS NULL AND ? IS NULL))
		LIMIT 1`
	err := r.db.QueryRowContext(ctx, checkQ, provider, mismatchType, paymentID, paymentID).Scan(&existingID)
	if err == nil {
		// Already logged
		return
	}

	mismatchID := model.NewULID()
	detailsBytes, _ := json.Marshal(details)

	insertQ := `
		INSERT INTO reconciliation_items (id, payment_id, provider, provider_reference, mismatch_type, details, created_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = r.db.ExecContext(ctx, insertQ, mismatchID, paymentID, provider, provRef, mismatchType, detailsBytes, now)
	if err != nil {
		r.logger.Error("failed to record reconciliation mismatch", "err", err)
		return
	}

	report.MismatchesFound++
	report.Items = append(report.Items, ReconciliationItem{
		ID:                mismatchID,
		PaymentID:         paymentID,
		Provider:          provider,
		ProviderReference: provRef,
		MismatchType:      mismatchType,
		Details:           detailsBytes,
		CreatedAt:         now,
	})
}

// GetReconciliationItems returns list of recorded reconciliation mismatches
func (r *ReconciliationService) GetReconciliationItems(ctx context.Context, onlyUnresolved bool) ([]ReconciliationItem, error) {
	query := `
		SELECT id, payment_id, provider, provider_reference, mismatch_type, details, resolved_at, created_at 
		FROM reconciliation_items`
	if onlyUnresolved {
		query += ` WHERE resolved_at IS NULL`
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ReconciliationItem
	for rows.Next() {
		var item ReconciliationItem
		var payID, provRef sql.NullString
		var detailsBytes []byte
		var resAt sql.NullTime

		if err := rows.Scan(&item.ID, &payID, &item.Provider, &provRef, &item.MismatchType, &detailsBytes, &resAt, &item.CreatedAt); err != nil {
			return nil, err
		}

		if payID.Valid {
			item.PaymentID = &payID.String
		}
		if provRef.Valid {
			item.ProviderReference = &provRef.String
		}
		if resAt.Valid {
			item.ResolvedAt = &resAt.Time
		}
		item.Details = detailsBytes
		items = append(items, item)
	}
	return items, rows.Err()
}

// ResolveReconciliationItem marks a mismatch as resolved
func (r *ReconciliationService) ResolveReconciliationItem(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `UPDATE reconciliation_items SET resolved_at = ? WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("reconciliation item %s not found", id)
	}
	return nil
}
