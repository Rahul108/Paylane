package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	"paylane-jwe"
	"payment-core/internal/adapter"
	"payment-core/internal/config"
	"payment-core/internal/handler"
	"payment-core/internal/outbox"
	"payment-core/internal/service"
)

const serviceName = "payment-core"
const defaultPort = "4011"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load(serviceName, defaultPort)
	logger.Info("starting service", "service", cfg.ServiceName, "port", cfg.Port, "keysDir", cfg.KeysDir)

	// Load service keys
	signKeyPath := filepath.Join(cfg.KeysDir, serviceName, "sign_private.pem")
	signKey, err := jwe.LoadRSAPrivateKey(signKeyPath)
	if err != nil {
		logger.Error("failed to load signing private key", "path", signKeyPath, "err", err)
		os.Exit(1)
	}

	encKeyPath := filepath.Join(cfg.KeysDir, serviceName, "enc_private.pem")
	encKey, err := jwe.LoadRSAPrivateKey(encKeyPath)
	if err != nil {
		logger.Error("failed to load encryption private key", "path", encKeyPath, "err", err)
		os.Exit(1)
	}

	registry := jwe.NewKeyRegistry(cfg.KeysDir)
	replay := jwe.NewMemoryReplayProtector()

	// Initialize database
	var db *sql.DB
	if cfg.DatabaseURL != "" {
		var err error
		db, err = waitForDB(cfg.DatabaseURL, 30*time.Second, logger)
		if err != nil {
			logger.Error("failed to connect to database", "err", err)
			os.Exit(1)
		}
		defer db.Close()

		if err := runMigrations(db, "migrations", logger); err != nil {
			logger.Error("failed to run migrations", "err", err)
			os.Exit(1)
		}
	}

	// Adapter registry
	mockMFSURL := os.Getenv("MOCK_MFS_URL")
	if mockMFSURL == "" {
		mockMFSURL = "http://mock-mfs:5011"
	}
	mockCardURL := os.Getenv("MOCK_CARD_URL")
	if mockCardURL == "" {
		mockCardURL = "http://mock-card:5012"
	}

	adapters := adapter.NewRegistry()
	adapters.Register("mock-mfs", adapter.NewMFSAdapter(mockMFSURL))
	adapters.Register("mfs", adapter.NewMFSAdapter(mockMFSURL))
	adapters.Register("mock-card", adapter.NewCardAdapter(mockCardURL))
	adapters.Register("card", adapter.NewCardAdapter(mockCardURL))

	// Payment service
	paymentSvc := service.NewPaymentService(db, adapters)

	// Timeout & Reconciliation services
	timeoutSvc := service.NewTimeoutService(db, adapters, logger)
	reconcileSvc := service.NewReconciliationService(db, adapters, logger)

	// JWE client for outbox dispatch
	jweClient := jwe.NewClient(serviceName, signKey, encKey, registry, replay, nil)

	// Transactional Outbox worker
	orchestratorURL := os.Getenv("ORCHESTRATOR_URL")
	if orchestratorURL == "" {
		orchestratorURL = "http://orchestrator:4010/events"
	}
	outboxWorker := outbox.NewWorker(db, orchestratorURL, jweClient, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if db != nil {
		outboxWorker.Start(ctx)
		defer outboxWorker.Stop()

		// Periodic background timeout sweep: check every 15s for payments stuck > 5m
		timeoutSvc.Start(ctx, 15*time.Second, 5*time.Minute)
		defer timeoutSvc.Stop()
	}

	// Handlers
	healthHandler := handler.NewHealthHandler(cfg.ServiceName, signKey, registry)
	paymentHandler := handler.NewPaymentHandler(cfg.ServiceName, signKey, registry, paymentSvc)
	reconcileHandler := handler.NewReconciliationHandler(cfg.ServiceName, signKey, registry, reconcileSvc, timeoutSvc)

	// Routes
	mux := http.NewServeMux()

	// 1. Unsigned liveness endpoint for Docker healthcheck
	mux.HandleFunc("GET /livez", healthHandler.Livez)

	// 2. JWE Authentication middleware for service mesh endpoints
	jweAuth := jwe.Middleware(cfg.ServiceName, encKey, registry, replay)

	// Health check over JWE
	mux.Handle("POST /health", jweAuth(http.HandlerFunc(healthHandler.Health)))

	// Core Payment APIs (Secured by JWE)
	mux.Handle("POST /payments", jweAuth(http.HandlerFunc(paymentHandler.InitiatePayment)))
	mux.Handle("GET /payments/{id}", jweAuth(http.HandlerFunc(paymentHandler.GetPayment)))
	mux.Handle("GET /payments/{id}/events", jweAuth(http.HandlerFunc(paymentHandler.GetPaymentEvents)))
	mux.Handle("GET /payments/{id}/ledger", jweAuth(http.HandlerFunc(paymentHandler.GetPaymentLedger)))
	mux.Handle("POST /payments/{id}/refunds", jweAuth(http.HandlerFunc(paymentHandler.CreateRefund)))
	mux.Handle("GET /payments/{id}/refunds", jweAuth(http.HandlerFunc(paymentHandler.GetPaymentRefunds)))

	// Customer Bindings APIs (Secured by JWE)
	mux.Handle("POST /bindings/initiate", jweAuth(http.HandlerFunc(paymentHandler.InitiateBinding)))
	mux.Handle("POST /bindings/confirm", jweAuth(http.HandlerFunc(paymentHandler.ConfirmBinding)))
	mux.Handle("GET /customers/{customer_id}/bindings", jweAuth(http.HandlerFunc(paymentHandler.GetCustomerBindings)))
	mux.Handle("POST /bindings/{id}/unbind", jweAuth(http.HandlerFunc(paymentHandler.Unbind)))

	// Timeouts & Reconciliation APIs (Secured by JWE)
	mux.Handle("POST /timeouts/sweep", jweAuth(http.HandlerFunc(reconcileHandler.SweepTimeouts)))
	mux.Handle("POST /reconciliation/run", jweAuth(http.HandlerFunc(reconcileHandler.RunReconciliation)))
	mux.Handle("GET /reconciliation/items", jweAuth(http.HandlerFunc(reconcileHandler.GetReconciliationItems)))
	mux.Handle("POST /reconciliation/items/{id}/resolve", jweAuth(http.HandlerFunc(reconcileHandler.ResolveItem)))

	// Webhooks from PGWs (Accepts JWE authenticated webhooks)
	mux.Handle("POST /webhooks/{provider}", jweAuth(http.HandlerFunc(paymentHandler.HandleWebhook)))

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	logger.Info("service listening", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped with error", "err", err)
		os.Exit(1)
	}
}

func waitForDB(dsn string, timeout time.Duration, logger *slog.Logger) (*sql.DB, error) {
	deadline := time.Now().Add(timeout)
	for {
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			if pingErr := db.Ping(); pingErr == nil {
				logger.Info("connected to MySQL successfully")
				return db, nil
			}
			_ = db.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for mysql: %w", err)
		}
		logger.Info("waiting for MySQL to be ready...")
		time.Sleep(2 * time.Second)
	}
}

func runMigrations(db *sql.DB, dir string, logger *slog.Logger) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Info("no migrations directory found, skipping migrations")
		return nil
	}
	if err := goose.SetDialect("mysql"); err != nil {
		return err
	}
	logger.Info("applying goose database migrations...")
	return goose.Up(db, dir)
}
