-- +goose Up
ALTER TABLE mfs_sessions ADD COLUMN IF NOT EXISTS payment_id VARCHAR(64) NULL AFTER id;

-- +goose Down
ALTER TABLE mfs_sessions DROP COLUMN IF EXISTS payment_id;
