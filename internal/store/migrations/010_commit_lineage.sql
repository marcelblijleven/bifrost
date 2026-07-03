-- +goose Up

-- Commit lineage tracking. last_known_sha is the head of the tracked branch as
-- bifrost last saw it; every push webhook must chain onto it (its 'before'
-- equals last_known_sha). A push that does not chain and is not an ancestor
-- fast-forward means the branch history was rewritten (force push): the
-- application is blocked (head_state='blocked') until a human re-baselines the
-- head via the API/frontend.
ALTER TABLE applications ADD COLUMN IF NOT EXISTS last_known_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE applications ADD COLUMN IF NOT EXISTS head_state TEXT NOT NULL DEFAULT 'ok';
ALTER TABLE applications ADD COLUMN IF NOT EXISTS blocked_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE applications ADD COLUMN IF NOT EXISTS blocked_at TIMESTAMPTZ;

-- The 'before' SHA of the push webhook that created the run, for audit/UI.
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS parent_sha TEXT NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS parent_sha;
ALTER TABLE applications DROP COLUMN IF EXISTS blocked_at;
ALTER TABLE applications DROP COLUMN IF EXISTS blocked_reason;
ALTER TABLE applications DROP COLUMN IF EXISTS head_state;
ALTER TABLE applications DROP COLUMN IF EXISTS last_known_sha;
