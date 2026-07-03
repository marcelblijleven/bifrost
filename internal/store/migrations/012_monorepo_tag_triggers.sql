-- +goose Up

-- Monorepo support: multiple applications may share one repository, each
-- with its own path filters and tag prefix. Uniqueness moves to include the
-- application name so webhook fan-out can address each app individually.
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_provider_owner_repo_key;
ALTER TABLE applications ADD CONSTRAINT applications_provider_owner_repo_name_key
    UNIQUE (provider, owner, repo, name);

-- trigger_type selects what starts a pipeline run: 'push' (commits on the
-- tracked branch, the default) or 'tag' (a tag matching tag_pattern is
-- pushed). An application listens to one or the other, never both.
ALTER TABLE applications ADD COLUMN IF NOT EXISTS trigger_type TEXT NOT NULL DEFAULT 'push';

-- tag_pattern is the glob a pushed tag must match to trigger a tag-triggered
-- application (e.g. 'v*' or 'frontend-v*'). Empty for push-triggered apps.
ALTER TABLE applications ADD COLUMN IF NOT EXISTS tag_pattern TEXT NOT NULL DEFAULT '';

-- tag_prefix namespaces the tags the semver step reads and writes, so several
-- applications can release from one repository without colliding (e.g.
-- prefix 'frontend-' yields tags like 'frontend-v1.2.3'). Empty means the
-- whole tag namespace, preserving single-app behaviour.
ALTER TABLE applications ADD COLUMN IF NOT EXISTS tag_prefix TEXT NOT NULL DEFAULT '';

-- trigger_tag records the tag name that triggered a tag-triggered run. The
-- partial unique index makes tag runs idempotent across duplicate webhook
-- deliveries and lets a recreated tag (same name, different commit) be
-- detected and blocked. Push-triggered runs leave it empty; their computed
-- release tag lives in the existing 'tag' column and may legitimately repeat
-- across failed attempts.
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
