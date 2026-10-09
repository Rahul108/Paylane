-- +goose Up
-- SQL in this section is executed after the migration is applied.

CREATE TABLE IF NOT EXISTS payments (
    id CHAR(26) NOT NULL PRIMARY KEY,
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    customer_id VARCHAR(64) NOT NULL,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'BDT',
    method VARCHAR(32) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'CREATED',
    failure_reason VARCHAR(255) NULL,
    provider_reference VARCHAR(128) NULL,
    metadata JSON NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_payments_customer (customer_id),
    INDEX idx_payments_status_updated (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS payment_events (
    id CHAR(26) NOT NULL PRIMARY KEY,
    payment_id CHAR(26) NOT NULL,
    from_status VARCHAR(32) NOT NULL,
    to_status VARCHAR(32) NOT NULL,
    reason VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_events_payment (payment_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ledger_entries (
    id CHAR(26) NOT NULL PRIMARY KEY,
    payment_id CHAR(26) NOT NULL,
    entry_type VARCHAR(16) NOT NULL, -- DEBIT, CREDIT
    account VARCHAR(64) NOT NULL,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'BDT',
    reason VARCHAR(128) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_ledger_payment (payment_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS refunds (
    id CHAR(26) NOT NULL PRIMARY KEY,
    payment_id CHAR(26) NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'BDT',
    status VARCHAR(32) NOT NULL,
    reason VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_refunds_payment (payment_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bindings (
    id CHAR(26) NOT NULL PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    method VARCHAR(32) NOT NULL, -- mfs, card
    provider VARCHAR(32) NOT NULL,
    token_reference VARCHAR(128) NOT NULL,
    masked_identifier VARCHAR(64) NOT NULL, -- e.g. 017*** or 411111****1111
    card_brand VARCHAR(32) NULL,
    card_last4 CHAR(4) NULL,
    expiry_month INT NULL,
    expiry_year INT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'BOUND', -- BOUND, UNBOUND
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_bindings_customer (customer_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS webhook_events (
    id CHAR(26) NOT NULL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    provider_event_id VARCHAR(128) NOT NULL,
    payload JSON NOT NULL,
    received_at DATETIME(6) NOT NULL,
    processed_at DATETIME(6) NULL,
    UNIQUE KEY uq_provider_event (provider, provider_event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS outbox (
    id CHAR(26) NOT NULL PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(64) NOT NULL,
    payload JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING', -- PENDING, PUBLISHED, FAILED
    retry_count INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_outbox_poll (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS reconciliation_items (
    id CHAR(26) NOT NULL PRIMARY KEY,
    payment_id CHAR(26) NULL,
    provider VARCHAR(32) NOT NULL,
    provider_reference VARCHAR(128) NULL,
    mismatch_type VARCHAR(64) NOT NULL,
    details JSON NOT NULL,
    resolved_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
DROP TABLE IF EXISTS reconciliation_items;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS bindings;
DROP TABLE IF EXISTS refunds;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS payment_events;
DROP TABLE IF EXISTS payments;
