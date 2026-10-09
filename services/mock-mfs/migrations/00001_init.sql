-- +goose Up
CREATE TABLE IF NOT EXISTS mfs_sessions (
    id CHAR(26) NOT NULL PRIMARY KEY,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'BDT',
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    msisdn VARCHAR(32) NULL,
    callback_url VARCHAR(255) NOT NULL,
    return_url VARCHAR(255) NOT NULL,
    scenario_override VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_mfs_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS mfs_agreements (
    id CHAR(26) NOT NULL PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    msisdn VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS mfs_agreements;
DROP TABLE IF EXISTS mfs_sessions;
