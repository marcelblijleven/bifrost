-- +goose Up
ALTER TABLE applications ADD COLUMN IF NOT EXISTS skip_conditions JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE applications DROP COLUMN IF EXISTS skip_conditions;
