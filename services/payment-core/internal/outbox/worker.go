package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"paylane-jwe"
)

type Worker struct {
	db             *sql.DB
	logger         *slog.Logger
	subscriberURL  string
	jweClient      *jwe.Client
	pollInterval   time.Duration
	stopCh         chan struct{}
}

func NewWorker(db *sql.DB, subscriberURL string, jweClient *jwe.Client, logger *slog.Logger) *Worker {
	return &Worker{
		db:            db,
		logger:        logger,
		subscriberURL: subscriberURL,
		jweClient:     jweClient,
		pollInterval:  2 * time.Second,
		stopCh:        make(chan struct{}),
	}
}

func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.processPendingEvents(ctx)
			}
		}
	}()
}

func (w *Worker) Stop() {
	close(w.stopCh)
}

func (w *Worker) processPendingEvents(ctx context.Context) {
	now := time.Now().UTC()
	query := `
		SELECT id, event_type, aggregate_id, payload, retry_count 
		FROM outbox 
		WHERE status = 'PENDING' AND next_retry_at <= ? 
		ORDER BY next_retry_at ASC 
		LIMIT 25`
	
	rows, err := w.db.QueryContext(ctx, query, now)
	if err != nil {
		w.logger.Error("failed to query pending outbox events", "err", err)
		return
	}
	defer rows.Close()

	type outboxRow struct {
		id          string
		eventType   string
		aggregateID string
		payload     []byte
		retryCount  int
	}

	var events []outboxRow
	for rows.Next() {
		var ev outboxRow
		if err := rows.Scan(&ev.id, &ev.eventType, &ev.aggregateID, &ev.payload, &ev.retryCount); err != nil {
			w.logger.Error("failed to scan outbox row", "err", err)
			continue
		}
		events = append(events, ev)
	}

	for _, ev := range events {
		w.dispatch(ctx, ev.id, ev.eventType, ev.aggregateID, ev.payload, ev.retryCount)
	}
}

func (w *Worker) dispatch(ctx context.Context, id, eventType, aggregateID string, payload []byte, retryCount int) {
	now := time.Now().UTC()

	// If a subscriber URL and JWE client is configured, dispatch over JWE
	var dispatchErr error
	if w.subscriberURL != "" && w.jweClient != nil {
		code, respBytes, err := w.jweClient.Post(ctx, "orchestrator", w.subscriberURL, payload, nil)
		if err != nil {
			dispatchErr = err
		} else if code >= 400 {
			dispatchErr = fmt.Errorf("subscriber returned HTTP %d: %s", code, string(respBytes))
		}
	}

	if dispatchErr == nil {
		// Mark published
		updateQuery := `UPDATE outbox SET status = 'PUBLISHED', updated_at = ? WHERE id = ?`
		if _, err := w.db.ExecContext(ctx, updateQuery, now, id); err != nil {
			w.logger.Error("failed to mark outbox event PUBLISHED", "id", id, "err", err)
		} else {
			w.logger.Info("outbox event published", "id", id, "type", eventType, "aggregate_id", aggregateID)
		}
	} else {
		// Calculate backoff
		newRetry := retryCount + 1
		backoffSeconds := 5 * (1 << min(newRetry, 5)) // 10s, 20s, 40s...
		nextRetry := now.Add(time.Duration(backoffSeconds) * time.Second)

		updateQuery := `UPDATE outbox SET retry_count = ?, next_retry_at = ?, updated_at = ? WHERE id = ?`
		_, _ = w.db.ExecContext(ctx, updateQuery, newRetry, nextRetry, now, id)
		w.logger.Warn("outbox dispatch failed, scheduled retry", "id", id, "type", eventType, "retry", newRetry, "err", dispatchErr)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
