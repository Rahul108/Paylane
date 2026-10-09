-- +goose Up
CREATE TABLE IF NOT EXISTS downstream_calls (
    id CHAR(26) NOT NULL PRIMARY KEY,
    call_type VARCHAR(32) NOT NULL, -- recharge, cashback, subscription
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    customer_id VARCHAR(64) NOT NULL,
    amount BIGINT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'SUCCEEDED',
    scenario_override VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_downstream_type (call_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS downstream_calls;
