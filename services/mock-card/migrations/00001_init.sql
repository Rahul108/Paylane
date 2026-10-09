-- +goose Up
CREATE TABLE IF NOT EXISTS card_sessions (
    id CHAR(26) NOT NULL PRIMARY KEY,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'BDT',
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    card_pan_masked VARCHAR(32) NULL,
    card_brand VARCHAR(32) NULL,
    token_id CHAR(26) NULL,
    callback_url VARCHAR(255) NOT NULL,
    return_url VARCHAR(255) NOT NULL,
    scenario_override VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_card_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS card_tokens (
    id CHAR(26) NOT NULL PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    card_brand VARCHAR(32) NOT NULL,
    card_last4 CHAR(4) NOT NULL,
    expiry_month INT NOT NULL,
    expiry_year INT NOT NULL,
    token_reference VARCHAR(128) NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS card_tokens;
DROP TABLE IF EXISTS card_sessions;
