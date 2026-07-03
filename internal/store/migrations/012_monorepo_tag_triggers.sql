-- +goose Up

-- Monorepo support: several applications may share one repository, so
-- uniqueness moves to include the application name.
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_provider_owner_repo_key;
ALTER TABLE applications ADD CONSTRAINT applications_provider_owner_repo_name_key
    UNIQUE (provider, owner, repo, name);

-- trigger_type: 'push' (commits on the tracked branch) or 'tag' (a tag
-- matching tag_pattern is pushed). tag_prefix namespaces the semver step's
-- tags per application (e.g. 'frontend-' → 'frontend-v1.2.3').
ALTER TABLE applications ADD COLUMN IF NOT EXISTS trigger_type TEXT NOT NULL DEFAULT 'push';
ALTER TABLE applications ADD COLUMN IF NOT EXISTS tag_pattern TEXT NOT NULL DEFAULT '';
ALTER TABLE applications ADD COLUMN IF NOT EXISTS tag_prefix TEXT NOT NULL DEFAULT '';

-- trigger_tag records the tag that triggered a tag-triggered run. The partial
-- unique index makes tag runs idempotent across duplicate deliveries and lets
-- a recreated tag be detected. Push runs leave it empty; their computed tag
-- (the 'tag' column) may legitimately repeat across failed attempts.
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS trigger_tag TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS pipeline_runs_app_trigger_tag_key
    ON pipeline_runs (application_id, trigger_tag) WHERE trigger_tag <> '';

-- +goose Down

DROP INDEX IF EXISTS pipeline_runs_app_trigger_tag_key;
ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS trigger_tag;
ALTER TABLE applications DROP COLUMN IF EXISTS tag_prefix;
ALTER TABLE applications DROP COLUMN IF EXISTS tag_pattern;
ALTER TABLE applications DROP COLUMN IF EXISTS trigger_type;
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_provider_owner_repo_name_key;
ALTER TABLE applications ADD CONSTRAINT applications_provider_owner_repo_key
    UNIQUE (provider, owner, repo);
