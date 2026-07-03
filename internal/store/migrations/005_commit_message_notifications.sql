-- +goose Up
ALTER TABLE pipeline_runs
    ADD COLUMN IF NOT EXISTS commit_message TEXT NOT NULL DEFAULT '';

ALTER TABLE applications
    ADD COLUMN IF NOT EXISTS notifications JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS commit_message;
ALTER TABLE applications  DROP COLUMN IF EXISTS notifications;
