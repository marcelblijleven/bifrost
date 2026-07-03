package postgres

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/marcelblijleven/bifrost/internal/store"
)

var _ store.Store = (*Store)(nil)

// Store implements store.Store using PostgreSQL via pgx/v5.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to PostgreSQL, runs pending migrations, and returns a Store.
func New(ctx context.Context, databaseURL string, migrations embed.FS) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// ── Applications ──────────────────────────────────────────────────────────────

const appColumns = `id, name, provider, owner, repo, branch, webhook_secret, pipeline_steps, notifications, skip_conditions, trigger_type, tag_pattern, tag_prefix, last_known_sha, head_state, blocked_reason, blocked_at, created_at, updated_at`

func scanApplication(row scanner) (*store.Application, error) {
	var a store.Application
	var stepsJSON []byte
	var notifsJSON []byte
	var skipJSON []byte
	if err := row.Scan(
		&a.ID, &a.Name, &a.Provider, &a.Owner, &a.Repo, &a.Branch,
		&a.WebhookSecret, &stepsJSON, &notifsJSON, &skipJSON,
		&a.TriggerType, &a.TagPattern, &a.TagPrefix,
		&a.LastKnownSHA, &a.HeadState, &a.BlockedReason, &a.BlockedAt,
		&a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(stepsJSON) > 0 {
		if err := json.Unmarshal(stepsJSON, &a.PipelineSteps); err != nil {
			return nil, fmt.Errorf("unmarshal pipeline_steps: %w", err)
		}
	}
	if len(notifsJSON) > 0 && string(notifsJSON) != "null" {
		if err := json.Unmarshal(notifsJSON, &a.Notifications); err != nil {
			return nil, fmt.Errorf("unmarshal notifications: %w", err)
		}
	}
	if len(skipJSON) > 0 && string(skipJSON) != "null" {
		if err := json.Unmarshal(skipJSON, &a.SkipConditions); err != nil {
			return nil, fmt.Errorf("unmarshal skip_conditions: %w", err)
		}
	}
	return &a, nil
}

func (s *Store) CreateApplication(ctx context.Context, a *store.Application) error {
	stepsJSON, err := json.Marshal(a.PipelineSteps)
	if err != nil {
		return fmt.Errorf("marshal pipeline_steps: %w", err)
	}
	notifsJSON, err := json.Marshal(a.Notifications)
	if err != nil {
		return fmt.Errorf("marshal notifications: %w", err)
	}
	skipJSON, err := json.Marshal(a.SkipConditions)
	if err != nil {
		return fmt.Errorf("marshal skip_conditions: %w", err)
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO applications (name, provider, owner, repo, branch, webhook_secret, pipeline_steps, notifications, skip_conditions, trigger_type, tag_pattern, tag_prefix)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+appColumns,
		a.Name, a.Provider, a.Owner, a.Repo, a.Branch, a.WebhookSecret, stepsJSON, notifsJSON, skipJSON,
		a.TriggerType, a.TagPattern, a.TagPrefix,
	)
	result, err := scanApplication(row)
	if err != nil {
		return fmt.Errorf("create application: %w", err)
	}
	*a = *result
	return nil
}

func (s *Store) GetApplication(ctx context.Context, id uuid.UUID) (*store.Application, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+appColumns+` FROM applications WHERE id=$1`, id)
	a, err := scanApplication(row)
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}
	return a, nil
}

func (s *Store) ListApplicationsByRepo(ctx context.Context, provider, owner, repo string) ([]*store.Application, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+appColumns+` FROM applications WHERE provider=$1 AND owner=$2 AND repo=$3 ORDER BY created_at ASC`,
		provider, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("list applications by repo: %w", err)
	}
	defer rows.Close()
	apps := make([]*store.Application, 0)
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Store) ListApplications(ctx context.Context) ([]*store.Application, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+appColumns+` FROM applications ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	apps := make([]*store.Application, 0)
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Store) ListApplicationsForUser(ctx context.Context, userID uuid.UUID) ([]*store.Application, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+appColumns+`
		FROM applications a
		WHERE
			NOT EXISTS (SELECT 1 FROM application_groups WHERE application_id = a.id)
			OR EXISTS (
				SELECT 1 FROM application_groups ag
				JOIN group_memberships gm ON ag.group_id = gm.group_id
				WHERE ag.application_id = a.id AND gm.user_id = $1
			)
		ORDER BY a.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list applications for user: %w", err)
	}
	defer rows.Close()
	apps := make([]*store.Application, 0)
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Store) CanUserAccessApplication(ctx context.Context, userID, appID uuid.UUID) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			NOT EXISTS (SELECT 1 FROM application_groups WHERE application_id = $1)
			OR EXISTS (
				SELECT 1 FROM application_groups ag
				JOIN group_memberships gm ON ag.group_id = gm.group_id
				WHERE ag.application_id = $1 AND gm.user_id = $2
			)`, appID, userID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check application access: %w", err)
	}
	return allowed, nil
}

func (s *Store) UpdateApplication(ctx context.Context, a *store.Application) error {
	stepsJSON, err := json.Marshal(a.PipelineSteps)
	if err != nil {
		return fmt.Errorf("marshal pipeline_steps: %w", err)
	}
	notifsJSON, err := json.Marshal(a.Notifications)
	if err != nil {
		return fmt.Errorf("marshal notifications: %w", err)
	}
	skipJSON, err := json.Marshal(a.SkipConditions)
	if err != nil {
		return fmt.Errorf("marshal skip_conditions: %w", err)
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE applications
		SET name=$2, provider=$3, owner=$4, repo=$5, branch=$6,
		    webhook_secret=CASE WHEN $7::text = '' THEN webhook_secret ELSE $7 END,
		    pipeline_steps=$8, notifications=$9, skip_conditions=$10,
		    trigger_type=$11, tag_pattern=$12, tag_prefix=$13, updated_at=NOW()
		WHERE id=$1
		RETURNING `+appColumns,
		a.ID, a.Name, a.Provider, a.Owner, a.Repo, a.Branch, a.WebhookSecret, stepsJSON, notifsJSON, skipJSON,
		a.TriggerType, a.TagPattern, a.TagPrefix,
	)
	result, err := scanApplication(row)
	if err != nil {
		return fmt.Errorf("update application: %w", err)
	}
	*a = *result
	return nil
}

func (s *Store) DeleteApplication(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, id)
	return err
}

// ── Commit lineage ────────────────────────────────────────────────────────────

func (s *Store) AdvanceApplicationHead(ctx context.Context, id uuid.UUID, from, to string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE applications
		SET last_known_sha = $3
		WHERE id = $1 AND last_known_sha = $2 AND head_state = 'ok'`,
		id, from, to,
	)
	if err != nil {
		return false, fmt.Errorf("advance application head: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) BlockApplication(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE applications
		SET head_state = 'blocked', blocked_reason = $2, blocked_at = NOW()
		WHERE id = $1`,
		id, reason,
	)
	return err
}

func (s *Store) AcceptApplicationHead(ctx context.Context, id uuid.UUID, head string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE applications
		SET last_known_sha = $2, head_state = 'ok', blocked_reason = '', blocked_at = NULL
		WHERE id = $1`,
		id, head,
	)
	return err
}

func (s *Store) CancelPendingRuns(ctx context.Context, applicationID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE pipeline_runs
		SET status = 'cancelled', completed_at = NOW()
		WHERE application_id = $1 AND status = 'pending'
		RETURNING id`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("cancel pending runs: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ── Application group access ──────────────────────────────────────────────────

func (s *Store) GrantGroupAccess(ctx context.Context, applicationID, groupID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO application_groups (application_id, group_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		applicationID, groupID,
	)
	return err
}

func (s *Store) RevokeGroupAccess(ctx context.Context, applicationID, groupID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM application_groups WHERE application_id=$1 AND group_id=$2`,
		applicationID, groupID,
	)
	return err
}

func (s *Store) ListApplicationGroups(ctx context.Context, applicationID uuid.UUID) ([]*store.Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, g.created_at
		FROM user_groups g
		JOIN application_groups ag ON ag.group_id = g.id
		WHERE ag.application_id = $1
		ORDER BY g.name`,
		applicationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list application groups: %w", err)
	}
	defer rows.Close()
	return scanGroups(rows)
}

// ── Pipeline runs ─────────────────────────────────────────────────────────────

const runColumns = `id, application_id, commit_sha, parent_sha, commit_message, branch, triggered_by, status, tag, trigger_tag, started_at, completed_at, created_at, released_at`

func scanRun(row scanner) (*store.PipelineRun, error) {
	var r store.PipelineRun
	if err := row.Scan(
		&r.ID, &r.ApplicationID, &r.CommitSHA, &r.ParentSHA, &r.CommitMessage, &r.Branch,
		&r.TriggeredBy, &r.Status, &r.Tag, &r.TriggerTag, &r.StartedAt, &r.CompletedAt, &r.CreatedAt, &r.ReleasedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreatePipelineRun(ctx context.Context, run *store.PipelineRun) error {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO pipeline_runs
		  (id, application_id, commit_sha, parent_sha, commit_message, branch, triggered_by, status, tag, trigger_tag, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+runColumns,
		run.ID, run.ApplicationID, run.CommitSHA, run.ParentSHA, run.CommitMessage, run.Branch, run.TriggeredBy, run.Status,
		run.Tag, run.TriggerTag, run.StartedAt, run.CompletedAt,
	)
	result, err := scanRun(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "pipeline_runs_app_trigger_tag_key" {
			return store.ErrDuplicateTriggerTag
		}
		return fmt.Errorf("create pipeline run: %w", err)
	}
	*run = *result
	return nil
}

func (s *Store) GetRunByTriggerTag(ctx context.Context, applicationID uuid.UUID, tag string) (*store.PipelineRun, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+runColumns+` FROM pipeline_runs WHERE application_id=$1 AND trigger_tag=$2`,
		applicationID, tag)
	r, err := scanRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run by trigger tag: %w", err)
	}
	return r, nil
}

func (s *Store) GetPipelineRun(ctx context.Context, id uuid.UUID) (*store.PipelineRun, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM pipeline_runs WHERE id=$1`, id)
	r, err := scanRun(row)
	if err != nil {
		return nil, fmt.Errorf("get pipeline run: %w", err)
	}
	return r, nil
}

func (s *Store) ListPipelineRuns(ctx context.Context, applicationID uuid.UUID, limit, offset int, filter store.RunFilter) ([]*store.PipelineRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+runColumns+` FROM pipeline_runs
		WHERE application_id=$1
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR branch = $5)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		applicationID, limit, offset, filter.Status, filter.Branch,
	)
	if err != nil {
		return nil, fmt.Errorf("list pipeline runs: %w", err)
	}
	defer rows.Close()
	runs := make([]*store.PipelineRun, 0)
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pipeline run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// leaseDuration is how long a claimed run stays owned without a heartbeat.
// Heartbeats (sent every leaseDuration/3) extend it; once it expires the
// reaper resets the run to pending so another instance can resume it.
const leaseDuration = 60 * time.Second

func (s *Store) ClaimPendingRun(ctx context.Context, instanceID string) (*store.PipelineRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		SELECT `+runColumns+` FROM pipeline_runs
		WHERE status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM pipeline_runs active
			WHERE active.application_id = pipeline_runs.application_id
			AND   active.status = 'running'
		)
		AND NOT EXISTS (
			SELECT 1 FROM applications a
			WHERE a.id = pipeline_runs.application_id
			AND   a.head_state = 'blocked'
		)
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`)
	run, err := scanRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim pending run: %w", err)
	}

	// Serialise claims per application across instances. Without this, two
	// instances can concurrently claim two different pending runs of the same
	// application: under READ COMMITTED neither sees the other's uncommitted
	// 'running' row, and SKIP LOCKED silently skips it.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, run.ApplicationID,
	); err != nil {
		return nil, fmt.Errorf("acquire application claim lock: %w", err)
	}
	// Re-check with a fresh snapshot now that we hold the application lock.
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pipeline_runs
			WHERE application_id = $1 AND status = 'running'
		)`, run.ApplicationID,
	).Scan(&active); err != nil {
		return nil, fmt.Errorf("recheck active run: %w", err)
	}
	if active {
		// Another instance claimed a run for this application first; leave
		// this one pending. It is picked up once the active run finishes.
		return nil, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = 'running', claimed_by = $2, lease_expires_at = NOW() + $3
		WHERE id = $1`, run.ID, instanceID, leaseDuration,
	); err != nil {
		return nil, fmt.Errorf("mark run claimed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}
	run.Status = "running"
	return run, nil
}

func (s *Store) HeartbeatRun(ctx context.Context, id uuid.UUID, instanceID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pipeline_runs
		SET lease_expires_at = NOW() + $3
		WHERE id = $1 AND claimed_by = $2 AND status = 'running'`,
		id, instanceID, leaseDuration,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ReapExpiredRuns(ctx context.Context) (int, error) {
	// lease_expires_at IS NULL covers runs left 'running' by a version of
	// bifrost that pre-dates leases.
	tag, err := s.pool.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = 'pending', claimed_by = '', lease_expires_at = NULL, completed_at = NULL
		WHERE status = 'running'
		AND (lease_expires_at IS NULL OR lease_expires_at < NOW())`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CancelRun(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pipeline_runs
		SET status='cancelled', completed_at=NOW()
		WHERE id=$1 AND status IN ('pending','running')`, id)
	return err
}

func (s *Store) ResetRunToPending(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = 'pending', claimed_by = '', lease_expires_at = NULL, completed_at = NULL
		WHERE id = $1`, id)
	return err
}

func (s *Store) UpdatePipelineRun(ctx context.Context, run *store.PipelineRun) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pipeline_runs SET status=$2, started_at=$3, completed_at=$4 WHERE id=$1`,
		run.ID, run.Status, run.StartedAt, run.CompletedAt,
	)
	return err
}

func (s *Store) UpdateRunTag(ctx context.Context, id uuid.UUID, tag string) error {
	_, err := s.pool.Exec(ctx, `UPDATE pipeline_runs SET tag=$2 WHERE id=$1`, id, tag)
	return err
}

func (s *Store) MarkRunReleased(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE pipeline_runs SET released_at=NOW() WHERE id=$1`, id)
	return err
}

func (s *Store) ListLatestRuns(ctx context.Context) (map[uuid.UUID]*store.PipelineRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (application_id) `+runColumns+`
		FROM pipeline_runs
		ORDER BY application_id, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list latest runs: %w", err)
	}
	defer rows.Close()
	latest := make(map[uuid.UUID]*store.PipelineRun)
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan latest run: %w", err)
		}
		latest[r.ApplicationID] = r
	}
	return latest, rows.Err()
}

func (s *Store) GetLastReleasedRun(ctx context.Context, applicationID uuid.UUID, branch string) (*store.PipelineRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+runColumns+` FROM pipeline_runs
		WHERE application_id=$1 AND branch=$2 AND released_at IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 1`,
		applicationID, branch,
	)
	r, err := scanRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last released run: %w", err)
	}
	return r, nil
}

// ── Step results ──────────────────────────────────────────────────────────────

const stepResultColumns = `id, run_id, step_name, step_index, status, output, error_message, external_run_id, overridden_by, override_reason, overridden_at, started_at, completed_at`

func scanStepResult(row scanner) (*store.StepResult, error) {
	var r store.StepResult
	if err := row.Scan(
		&r.ID, &r.RunID, &r.StepName, &r.StepIndex,
		&r.Status, &r.Output, &r.ErrorMessage, &r.ExternalRunID,
		&r.OverriddenBy, &r.OverrideReason, &r.OverriddenAt,
		&r.StartedAt, &r.CompletedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListStepResults(ctx context.Context, runID uuid.UUID) ([]*store.StepResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+stepResultColumns+`
		FROM pipeline_step_results
		WHERE run_id=$1
		ORDER BY step_index`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("list step results: %w", err)
	}
	defer rows.Close()
	results := make([]*store.StepResult, 0)
	for rows.Next() {
		r, err := scanStepResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) GetStepResultByExternalRunID(ctx context.Context, externalRunID int64) (*store.StepResult, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+stepResultColumns+`
		FROM pipeline_step_results
		WHERE external_run_id=$1
		LIMIT 1`,
		externalRunID,
	)
	r, err := scanStepResult(row)
	if err != nil {
		return nil, fmt.Errorf("get step result by external run id: %w", err)
	}
	return r, nil
}

func (s *Store) CreateStepResult(ctx context.Context, r *store.StepResult) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pipeline_step_results (id, run_id, step_name, step_index, status, output, error_message, external_run_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, r.RunID, r.StepName, r.StepIndex, r.Status, r.Output, r.ErrorMessage, r.ExternalRunID,
	)
	return err
}

func (s *Store) OverrideStepResult(ctx context.Context, runID uuid.UUID, stepIndex int, by, reason string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pipeline_step_results
		SET status='overridden', overridden_by=$3, override_reason=$4, overridden_at=NOW()
		WHERE run_id=$1 AND step_index=$2 AND status='failed'`,
		runID, stepIndex, by, reason,
	)
	if err != nil {
		return false, fmt.Errorf("override step result: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) SetStepResultExternalRunID(ctx context.Context, runID uuid.UUID, stepIndex int, externalRunID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pipeline_step_results
		SET external_run_id=$3
		WHERE run_id=$1 AND step_index=$2`,
		runID, stepIndex, externalRunID,
	)
	return err
}

func (s *Store) DeleteStepResultsFrom(ctx context.Context, runID uuid.UUID, fromStepIndex int) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM pipeline_step_results WHERE run_id=$1 AND step_index>=$2`,
		runID, fromStepIndex,
	)
	return err
}

func (s *Store) ResetStepResultsFrom(ctx context.Context, runID uuid.UUID, fromStepIndex int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pipeline_step_results
		SET status='pending', error_message='', output='', external_run_id=NULL,
		    overridden_by='', override_reason='', overridden_at=NULL,
		    started_at=NULL, completed_at=NULL
		WHERE run_id=$1 AND step_index>=$2`,
		runID, fromStepIndex,
	)
	return err
}

func (s *Store) UpdateStepResult(ctx context.Context, r *store.StepResult) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pipeline_step_results
		SET status=$2, output=$3, error_message=$4, external_run_id=$5, started_at=$6, completed_at=$7
		WHERE id=$1`,
		r.ID, r.Status, r.Output, r.ErrorMessage, r.ExternalRunID, r.StartedAt, r.CompletedAt,
	)
	return err
}

// ── Users ─────────────────────────────────────────────────────────────────────

func (s *Store) CreateUser(ctx context.Context, u *store.User) error {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_admin) VALUES ($1, $2, $3)
		RETURNING id, email, is_admin, created_at`, u.Email, u.PasswordHash, u.IsAdmin)
	return row.Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, is_admin, created_at FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserForAuth(ctx context.Context, email string) (*store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, is_admin, password_hash, created_at FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.IsAdmin, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]*store.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, email, is_admin, created_at FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]*store.User, 0)
	for rows.Next() {
		var u store.User
		if err := rows.Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE is_admin`).Scan(&n)
	return n, err
}

func (s *Store) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, passwordHash, id)
	return err
}

func (s *Store) SetUserAdmin(ctx context.Context, id uuid.UUID, isAdmin bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET is_admin=$1 WHERE id=$2`, isAdmin, id)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}

// ── Groups ────────────────────────────────────────────────────────────────────

func scanGroups(rows pgx.Rows) ([]*store.Group, error) {
	groups := make([]*store.Group, 0)
	for rows.Next() {
		var g store.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, &g)
	}
	return groups, rows.Err()
}

func (s *Store) CreateGroup(ctx context.Context, g *store.Group) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO user_groups (name) VALUES ($1)
		RETURNING id, name, created_at`, g.Name).
		Scan(&g.ID, &g.Name, &g.CreatedAt)
}

func (s *Store) GetGroup(ctx context.Context, id uuid.UUID) (*store.Group, error) {
	var g store.Group
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM user_groups WHERE id=$1`, id).
		Scan(&g.ID, &g.Name, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) ListGroups(ctx context.Context) ([]*store.Group, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, created_at FROM user_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (s *Store) UpdateGroup(ctx context.Context, g *store.Group) error {
	_, err := s.pool.Exec(ctx, `UPDATE user_groups SET name=$1 WHERE id=$2`, g.Name, g.ID)
	return err
}

func (s *Store) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_groups WHERE id=$1`, id)
	return err
}

func (s *Store) AddUserToGroup(ctx context.Context, userID, groupID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO group_memberships (user_id, group_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		userID, groupID,
	)
	return err
}

func (s *Store) RemoveUserFromGroup(ctx context.Context, userID, groupID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM group_memberships WHERE user_id=$1 AND group_id=$2`,
		userID, groupID,
	)
	return err
}

func (s *Store) ListGroupMembers(ctx context.Context, groupID uuid.UUID) ([]*store.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM users u
		JOIN group_memberships gm ON gm.user_id = u.id
		WHERE gm.group_id = $1
		ORDER BY u.email`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]*store.User, 0)
	for rows.Next() {
		var u store.User
		if err := rows.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// ── Approval requests ─────────────────────────────────────────────────────────

const approvalColumns = `id, run_id, step_name, step_index, status, resolved_by, message, created_at, resolved_at, superseded_by`

func scanApproval(row scanner) (*store.ApprovalRequest, error) {
	var r store.ApprovalRequest
	if err := row.Scan(
		&r.ID, &r.RunID, &r.StepName, &r.StepIndex,
		&r.Status, &r.ResolvedBy, &r.Message, &r.CreatedAt, &r.ResolvedAt, &r.SupersededBy,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateApprovalRequest(ctx context.Context, r *store.ApprovalRequest) error {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO approval_requests (id, run_id, step_name, step_index, message)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+approvalColumns,
		r.ID, r.RunID, r.StepName, r.StepIndex, r.Message,
	)
	result, err := scanApproval(row)
	if err != nil {
		return fmt.Errorf("create approval request: %w", err)
	}
	*r = *result
	return nil
}

func (s *Store) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*store.ApprovalRequest, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+approvalColumns+` FROM approval_requests WHERE id=$1`, id)
	r, err := scanApproval(row)
	if err != nil {
		return nil, fmt.Errorf("get approval request: %w", err)
	}
	return r, nil
}

func (s *Store) GetPendingApproval(ctx context.Context, runID uuid.UUID, stepIndex int) (*store.ApprovalRequest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+approvalColumns+` FROM approval_requests
		WHERE run_id=$1 AND step_index=$2 AND status='pending'
		ORDER BY created_at DESC LIMIT 1`,
		runID, stepIndex,
	)
	r, err := scanApproval(row)
	if err != nil {
		return nil, fmt.Errorf("get pending approval: %w", err)
	}
	return r, nil
}

func (s *Store) GetApprovalForStep(ctx context.Context, runID uuid.UUID, stepIndex int) (*store.ApprovalRequest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+approvalColumns+` FROM approval_requests
		WHERE run_id=$1 AND step_index=$2
		ORDER BY created_at DESC LIMIT 1`,
		runID, stepIndex,
	)
	r, err := scanApproval(row)
	if err != nil {
		return nil, fmt.Errorf("get approval for step: %w", err)
	}
	return r, nil
}

func (s *Store) DeleteApprovalRequestsFrom(ctx context.Context, runID uuid.UUID, fromStepIndex int) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM approval_requests WHERE run_id=$1 AND step_index>=$2`,
		runID, fromStepIndex,
	)
	return err
}

func (s *Store) ResolveApprovalRequest(ctx context.Context, id uuid.UUID, status, resolvedBy string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		UPDATE approval_requests
		SET status=$2, resolved_by=$3, resolved_at=$4
		WHERE id=$1 AND status='pending'`,
		id, status, resolvedBy, now,
	)
	return err
}

// SupersedeOlderApprovals marks pending approval requests for the same application
// and step index that belong to runs created before approvedRunID as superseded.
// Returns the run IDs that were superseded so the caller can update their status.
func (s *Store) SupersedeOlderApprovals(ctx context.Context, applicationID uuid.UUID, stepIndex int, approvedRunID uuid.UUID) ([]uuid.UUID, error) {
	now := time.Now()
	rows, err := s.pool.Query(ctx, `
		UPDATE approval_requests ar
		SET status = 'superseded', resolved_at = $1, superseded_by = $2
		FROM pipeline_runs pr
		WHERE ar.run_id = pr.id
		  AND pr.application_id = $3
		  AND ar.step_index = $4
		  AND ar.status = 'pending'
		  AND pr.id != $2
		  AND pr.created_at < (SELECT created_at FROM pipeline_runs WHERE id = $2)
		RETURNING ar.run_id`,
		now, approvedRunID, applicationID, stepIndex,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		runIDs = append(runIDs, id)
	}
	return runIDs, rows.Err()
}

func (s *Store) ListApprovalRequests(ctx context.Context, runID uuid.UUID) ([]*store.ApprovalRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+approvalColumns+` FROM approval_requests
		WHERE run_id=$1 ORDER BY step_index`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("list approval requests: %w", err)
	}
	defer rows.Close()
	reqs := make([]*store.ApprovalRequest, 0)
	for rows.Next() {
		r, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

func (s *Store) GetDashboardStats(ctx context.Context) (*store.DashboardStats, error) {
	stats := &store.DashboardStats{
		RunsByDay:      make([]store.DayStats, 0),
		RecentRuns:     make([]store.RecentRun, 0),
		PendingActions: make([]store.PendingAction, 0),
	}

	// Aggregate counts + average duration over the last 30 days.
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)                                                              AS total_runs,
			COUNT(*) FILTER (WHERE status = 'success')                           AS succeeded,
			COUNT(*) FILTER (WHERE status = 'failed')                            AS failed,
			COALESCE(AVG(
				EXTRACT(EPOCH FROM (completed_at - started_at))
			) FILTER (WHERE status = 'success' AND started_at IS NOT NULL AND completed_at IS NOT NULL), 0) AS avg_duration
		FROM pipeline_runs
		WHERE created_at >= NOW() - INTERVAL '30 days'`,
	).Scan(&stats.TotalRuns, &stats.SucceededRuns, &stats.FailedRuns, &stats.AvgDurationSeconds)
	if err != nil {
		return nil, fmt.Errorf("dashboard aggregates: %w", err)
	}

	// Runs per calendar day for the last 30 days (always 30 rows, even if zero).
	rows, err := s.pool.Query(ctx, `
		WITH days AS (
			SELECT generate_series(
				(NOW() - INTERVAL '29 days')::date,
				NOW()::date,
				'1 day'::interval
			)::date AS day
		)
		SELECT
			days.day::text,
			COALESCE(COUNT(r.id), 0)                                            AS total,
			COALESCE(COUNT(r.id) FILTER (WHERE r.status = 'success'), 0)        AS succeeded,
			COALESCE(COUNT(r.id) FILTER (WHERE r.status = 'failed'), 0)         AS failed
		FROM days
		LEFT JOIN pipeline_runs r ON r.created_at::date = days.day
		GROUP BY days.day
		ORDER BY days.day`)
	if err != nil {
		return nil, fmt.Errorf("dashboard runs by day: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d store.DayStats
		if err := rows.Scan(&d.Date, &d.Total, &d.Succeeded, &d.Failed); err != nil {
			return nil, err
		}
		stats.RunsByDay = append(stats.RunsByDay, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 10 most recent runs across all applications.
	recentRows, err := s.pool.Query(ctx, `
		SELECT r.id, r.application_id, r.commit_sha, r.branch, r.triggered_by,
		       r.status, r.started_at, r.completed_at, r.created_at,
		       a.name AS application_name
		FROM pipeline_runs r
		JOIN applications a ON a.id = r.application_id
		ORDER BY r.created_at DESC
		LIMIT 10`)
	if err != nil {
		return nil, fmt.Errorf("dashboard recent runs: %w", err)
	}
	defer recentRows.Close()
	for recentRows.Next() {
		var rr store.RecentRun
		if err := recentRows.Scan(
			&rr.ID, &rr.ApplicationID, &rr.CommitSHA, &rr.Branch, &rr.TriggeredBy,
			&rr.Status, &rr.StartedAt, &rr.CompletedAt, &rr.CreatedAt,
			&rr.ApplicationName,
		); err != nil {
			return nil, err
		}
		stats.RecentRuns = append(stats.RecentRuns, rr)
	}
	if err := recentRows.Err(); err != nil {
		return nil, err
	}

	// Items needing human attention: pending approval gates + queued runs.
	actionRows, err := s.pool.Query(ctx, `
		SELECT run_id, application_id, application_name, type, message, created_at
		FROM (
			SELECT
				ar.run_id,
				r.application_id,
				a.name  AS application_name,
				'approval' AS type,
				ar.message,
				ar.created_at
			FROM approval_requests ar
			JOIN pipeline_runs  r ON r.id  = ar.run_id
			JOIN applications   a ON a.id  = r.application_id
			WHERE ar.status = 'pending'

			UNION ALL

			SELECT
				r.id,
				r.application_id,
				a.name AS application_name,
				'queued' AS type,
				'Waiting for active run to complete' AS message,
				r.created_at
			FROM pipeline_runs r
			JOIN applications  a ON a.id = r.application_id
			WHERE r.status = 'pending'
		) t
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("dashboard pending actions: %w", err)
	}
	defer actionRows.Close()
	for actionRows.Next() {
		var pa store.PendingAction
		if err := actionRows.Scan(
			&pa.RunID, &pa.ApplicationID, &pa.ApplicationName,
			&pa.Type, &pa.Message, &pa.CreatedAt,
		); err != nil {
			return nil, err
		}
		stats.PendingActions = append(stats.PendingActions, pa)
	}
	return stats, actionRows.Err()
}

// ErrNoRows is re-exported so callers don't need to import pgx directly.
var ErrNoRows = pgx.ErrNoRows
