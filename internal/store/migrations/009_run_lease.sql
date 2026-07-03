-- +goose Up

-- Ownership lease for running pipeline runs. claimed_by identifies the bifrost
-- instance executing the run; lease_expires_at is extended by a heartbeat while
-- the run executes. Runs whose lease expired are reset to pending by the reaper,
-- so a crashed instance's work is picked up without stealing healthy instances'
-- runs (safe with multiple replicas).
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS claimed_by TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_pipeline_runs_lease
    ON pipeline_runs (status, lease_expires_at);

-- +goose Down

DROP INDEX IF EXISTS idx_pipeline_runs_lease;
ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS claimed_by;
ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS lease_expires_at;
