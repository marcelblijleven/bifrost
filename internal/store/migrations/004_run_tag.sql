-- +goose Up

-- Store the computed version tag on the run so that retries can restore the
-- exact same tag rather than re-deriving it from the live git tag list.
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS tag TEXT NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS tag;
