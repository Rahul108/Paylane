-- +goose Up
ALTER TABLE card_sessions ADD COLUMN IF NOT EXISTS payment_id VARCHAR(64) NULL AFTER id;

-- +goose Down
ALTER TABLE card_sessions DROP COLUMN IF EXISTS payment_id;
