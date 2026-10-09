package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	"mock-card/internal/config"
	"mock-card/internal/handler"
	"mock-card/internal/service"
	"paylane-jwe"
)

const serviceName = "mock-card"
const defaultPort = "5012"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load(serviceName, defaultPort)
	logger.Info("starting service", "service", cfg.ServiceName, "port", cfg.Port, "keysDir", cfg.KeysDir)

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

	jweClient := jwe.NewClient(serviceName, signKey, encKey, registry, replay, nil)

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", cfg.Port)
	}
	cardService := service.NewService(db, jweClient, baseURL, logger)

	mux := http.NewServeMux()
	healthHandler := handler.NewHealthHandler(cfg.ServiceName, signKey, registry)
	cardHandler := handler.NewCardHandler(cardService)

	// Liveness
	mux.HandleFunc("GET /livez", healthHandler.Livez)

	// JWE Auth
	jweAuth := jwe.Middleware(cfg.ServiceName, encKey, registry, replay)
	mux.Handle("POST /health", jweAuth(http.HandlerFunc(healthHandler.Health)))

	// API sessions (accepts direct calls and JWE calls)
	mux.HandleFunc("POST /sessions", cardHandler.CreateSession)
	mux.HandleFunc("GET /sessions/{id}", cardHandler.GetSession)

	// Hosted Pages (browser access)
	mux.HandleFunc("GET /checkout", cardHandler.RenderCheckout)
	mux.HandleFunc("POST /checkout/confirm", cardHandler.ConfirmCheckout)

	// Settlement report
	mux.HandleFunc("GET /settlement", cardHandler.GetSettlement)

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
