-- +goose Up
-- SQL in this section is executed after the migration is applied.

CREATE TABLE IF NOT EXISTS journeys (
    id CHAR(26) NOT NULL PRIMARY KEY,
    journey_type VARCHAR(64) NOT NULL, -- ui_payment, uiless_payment, payment_recharge, etc.
    customer_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'STARTED',
    input_payload JSON NOT NULL,
    current_step VARCHAR(64) NULL,
    payment_id CHAR(26) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_journeys_customer (customer_id),
    INDEX idx_journeys_status_updated (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS journey_steps (
    id CHAR(26) NOT NULL PRIMARY KEY,
    journey_id CHAR(26) NOT NULL,
    step_name VARCHAR(64) NOT NULL,
    step_order INT NOT NULL DEFAULT 1,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    attempt_count INT NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    downstream_reference VARCHAR(128) NULL,
    request_summary JSON NULL,
    response_summary JSON NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_steps_journey (journey_id, step_order),
    INDEX idx_steps_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS journey_steps;
DROP TABLE IF EXISTS journeys;
