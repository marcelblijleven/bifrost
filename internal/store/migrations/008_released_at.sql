-- +goose Up

-- Marks the moment a run completed all of its steps successfully. Used as the
-- baseline for the changelog step: it diffs against the last released run's
-- tag (or commit SHA if no tag was created) instead of the latest Git tag.
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS released_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS released_at;
