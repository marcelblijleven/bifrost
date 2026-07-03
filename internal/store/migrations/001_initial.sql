-- +goose Up

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE applications (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        NOT NULL,
    provider       TEXT        NOT NULL,
    owner          TEXT        NOT NULL,
    repo           TEXT        NOT NULL,
    branch         TEXT        NOT NULL DEFAULT 'main',
    webhook_secret TEXT        NOT NULL,
    pipeline_steps JSONB       NOT NULL DEFAULT '[]',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, owner, repo)
);

CREATE TABLE pipeline_runs (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID        NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    commit_sha     TEXT        NOT NULL,
    branch         TEXT        NOT NULL,
    triggered_by   TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'pending',
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE pipeline_step_results (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID        NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    step_name     TEXT        NOT NULL,
    step_index    INT         NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    output        TEXT        NOT NULL DEFAULT '',
    error_message TEXT        NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ
);

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Named user_groups to avoid the SQL reserved word GROUP
CREATE TABLE user_groups (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE group_memberships (
    user_id  UUID NOT NULL REFERENCES users(id)       ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id)
);

CREATE TABLE application_groups (
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    group_id       UUID NOT NULL REFERENCES user_groups(id)  ON DELETE CASCADE,
    PRIMARY KEY (application_id, group_id)
);

-- Created when a pipeline hits an approval step; the pipeline goroutine polls
-- this table until the request is resolved or the timeout elapses.
CREATE TABLE approval_requests (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      UUID        NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    step_name   TEXT        NOT NULL,
    step_index  INT         NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending', -- pending, approved, rejected
    resolved_by TEXT        NOT NULL DEFAULT '',
    message     TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (run_id, step_index)
);

-- RLS: enabled on all tables. Permissive service-role policies are active now.
-- User-scoped policies (commented below) are ready to swap in once JWT auth
-- sets app.user_id per request via SET LOCAL.
ALTER TABLE applications      ENABLE ROW LEVEL SECURITY;
ALTER TABLE pipeline_runs     ENABLE ROW LEVEL SECURITY;
ALTER TABLE pipeline_step_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE users             ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_groups       ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE application_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_requests ENABLE ROW LEVEL SECURITY;

CREATE POLICY applications_service_access      ON applications           USING (true) WITH CHECK (true);
CREATE POLICY runs_service_access              ON pipeline_runs          USING (true) WITH CHECK (true);
CREATE POLICY step_results_service_access      ON pipeline_step_results  USING (true) WITH CHECK (true);
CREATE POLICY users_service_access             ON users                  USING (true) WITH CHECK (true);
CREATE POLICY groups_service_access            ON user_groups            USING (true) WITH CHECK (true);
CREATE POLICY memberships_service_access       ON group_memberships      USING (true) WITH CHECK (true);
CREATE POLICY app_groups_service_access        ON application_groups     USING (true) WITH CHECK (true);
CREATE POLICY approvals_service_access         ON approval_requests      USING (true) WITH CHECK (true);

-- Future user-scoped RLS (swap in when JWT middleware sets app.user_id):
--
-- DROP POLICY applications_service_access ON applications;
-- CREATE POLICY applications_group_access ON applications
--     USING (
--         EXISTS (
--             SELECT 1 FROM application_groups ag
--             JOIN group_memberships gm ON ag.group_id = gm.group_id
--             WHERE ag.application_id = id
--               AND gm.user_id = current_setting('app.user_id', true)::uuid
--         )
--     );

-- +goose Down

DROP TABLE IF EXISTS approval_requests;
DROP TABLE IF EXISTS application_groups;
DROP TABLE IF EXISTS group_memberships;
DROP TABLE IF EXISTS user_groups;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS pipeline_step_results;
DROP TABLE IF EXISTS pipeline_runs;
DROP TABLE IF EXISTS applications;
