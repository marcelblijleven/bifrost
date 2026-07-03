-- +goose Up

-- Manual override of a failed step (e.g. a dispatched workflow that concluded
-- 'failure' but a human decides the release may proceed anyway). The step's
-- status becomes 'overridden' and the run resumes from the next step. Who
-- overrode it and why are recorded for the audit trail; error_message keeps
-- the original failure.
ALTER TABLE pipeline_step_results ADD COLUMN IF NOT EXISTS overridden_by TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_step_results ADD COLUMN IF NOT EXISTS override_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_step_results ADD COLUMN IF NOT EXISTS overridden_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE pipeline_step_results DROP COLUMN IF EXISTS overridden_at;
ALTER TABLE pipeline_step_results DROP COLUMN IF EXISTS override_reason;
ALTER TABLE pipeline_step_results DROP COLUMN IF EXISTS overridden_by;
