package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrDuplicateTriggerTag is returned by CreatePipelineRun when a run for the
// same application and trigger tag already exists.
var ErrDuplicateTriggerTag = errors.New("a run for this trigger tag already exists")

// StepConfig is one step in an application's pipeline, stored as JSONB.
type StepConfig struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// NotificationConfig holds application-level notification settings.
type NotificationConfig struct {
	OnFailureURL  string            `json:"on_failure_url,omitempty"`
	OnApprovalURL string            `json:"on_approval_url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// SkipConditions define when incoming webhook pushes should be ignored.
// All conditions are optional; an empty SkipConditions never skips.
type SkipConditions struct {
	// CommitPatterns skips the run if the commit message contains any of these strings.
	CommitPatterns []string `json:"commit_patterns,omitempty"`
	// PathsIgnore skips the run when ALL changed files match at least one pattern.
	// Uses glob syntax; "**" matches across path separators.
	PathsIgnore []string `json:"paths_ignore,omitempty"`
	// PathsInclude skips the run when NO changed files match any pattern.
	// Use this to run the pipeline only for changes in specific directories.
	PathsInclude []string `json:"paths_include,omitempty"`
	// SkipBackfill, when true, disables backfilling runs for commits that were
	// skipped by a missed webhook. Bifrost then syncs straight to the pushed
	// head (the pre-backfill behaviour) instead of creating a run per missed
	// commit. Missed commits are still covered by the pushed head's run, whose
	// changelog derives from commits since the last tag.
	SkipBackfill bool `json:"skip_backfill,omitempty"`
}

// RunFilter narrows ListPipelineRuns results. Empty strings mean no filter.
type RunFilter struct {
	Status string
	Branch string
}

// Application head states. A blocked application accepts no new pipeline runs
// until a human re-baselines its branch head (see AcceptApplicationHead).
const (
	HeadStateOK      = "ok"
	HeadStateBlocked = "blocked"
)

// Application trigger types: an application runs its pipeline for commits on
// the tracked branch OR for pushed tags matching TagPattern, never both.
const (
	TriggerPush = "push"
	TriggerTag  = "tag"
)

// Application is a repository registered with bifrost.
type Application struct {
	ID             uuid.UUID
	Name           string
	Provider       string // "github"
	Owner          string
	Repo           string
	Branch         string
	WebhookSecret  string
	PipelineSteps  []StepConfig
	Notifications  NotificationConfig
	SkipConditions SkipConditions
	// TriggerType is TriggerPush (default) or TriggerTag.
	TriggerType string
	// TagPattern is the glob a pushed tag must match to trigger a run
	// (e.g. "v*"). Only set for tag-triggered applications.
	TagPattern string
	// TagPrefix namespaces the tags the semver step reads and creates
	// (e.g. "frontend-" yields "frontend-v1.2.3"), so several applications
	// can release from one repository.
	TagPrefix string
	// LastKnownSHA is the head of the tracked branch as bifrost last saw it.
	// Every incoming push must chain onto it; see Handler.HandleWebhook.
	LastKnownSHA string
	// HeadState is HeadStateOK or HeadStateBlocked (history rewrite detected).
	HeadState     string
	BlockedReason string
	BlockedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PipelineRun is one execution of an application's pipeline.
type PipelineRun struct {
	ID            uuid.UUID
	ApplicationID uuid.UUID
	CommitSHA     string
	// ParentSHA is the branch head before the push that triggered this run
	// (the webhook's 'before' field). Empty for runs created via the API.
	ParentSHA     string
	CommitMessage string
	Branch        string
	TriggeredBy   string
	Status        string // pending, running, success, failed, cancelled, superseded, skipped, blocked
	Tag           string // computed version tag set by the semver step, e.g. "v0.1.6"
	// TriggerTag is the tag that triggered this run (tag-triggered apps only).
	// Unique per application: duplicate deliveries are rejected by the store.
	TriggerTag  string
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	// ReleasedAt is set once the run completes all of its steps successfully.
	// Used by the changelog step as the baseline for the next run's diff.
	ReleasedAt *time.Time
}

// StepResult is the outcome of a single step within a PipelineRun.
type StepResult struct {
	ID            uuid.UUID
	RunID         uuid.UUID
	StepName      string
	StepIndex     int
	Status        string // pending, running, success, failed, skipped, cancelled, overridden
	Output        string
	ErrorMessage  string
	ExternalRunID *int64 // GitHub Actions workflow run ID for dispatch steps
	// Override audit trail: set when a human marked this failed step as
	// overridden so the run could resume. ErrorMessage keeps the original
	// failure.
	OverriddenBy   string
	OverrideReason string
	OverriddenAt   *time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
}

// User is a bifrost user.
type User struct {
	ID           uuid.UUID
	Email        string
	IsAdmin      bool
	PasswordHash string `json:"-"` // never serialised in API responses
	CreatedAt    time.Time
}

// Group is a named set of users that can be granted access to applications.
type Group struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// ApprovalRequest is created when a pipeline hits an approval step.
// The pipeline blocks until the request is resolved or times out.
type ApprovalRequest struct {
	ID           uuid.UUID
	RunID        uuid.UUID
	StepName     string
	StepIndex    int
	Status       string // pending, approved, rejected, superseded
	ResolvedBy   string
	Message      string
	CreatedAt    time.Time
	ResolvedAt   *time.Time
	SupersededBy *uuid.UUID // run ID that caused supersession
}

// PendingAction is an item that requires human attention: either an approval
// gate blocking a running pipeline, or a queued run waiting for another to finish.
type PendingAction struct {
	RunID           uuid.UUID `json:"run_id"`
	ApplicationID   uuid.UUID `json:"application_id"`
	ApplicationName string    `json:"application_name"`
	Type            string    `json:"type"` // "approval" | "queued"
	Message         string    `json:"message"`
	CreatedAt       time.Time `json:"created_at"`
}

// DashboardStats aggregates activity metrics for the dashboard page.
type DashboardStats struct {
	TotalRuns          int             `json:"total_runs"`
	SucceededRuns      int             `json:"succeeded_runs"`
	FailedRuns         int             `json:"failed_runs"`
	AvgDurationSeconds float64         `json:"avg_duration_seconds"`
	RunsByDay          []DayStats      `json:"runs_by_day"`
	RecentRuns         []RecentRun     `json:"recent_runs"`
	PendingActions     []PendingAction `json:"pending_actions"`
}

// DayStats is a single day bucket in the runs-over-time chart.
type DayStats struct {
	Date      string `json:"date"` // YYYY-MM-DD
	Total     int    `json:"total"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
}

// RecentRun is a pipeline run annotated with its application name, for the dashboard feed.
type RecentRun struct {
	PipelineRun
	ApplicationName string `json:"application_name"`
}

// Store is the persistence interface for bifrost.
type Store interface {
	// Applications
	CreateApplication(ctx context.Context, a *Application) error
	GetApplication(ctx context.Context, id uuid.UUID) (*Application, error)
	// ListApplicationsByRepo returns every application registered for the
	// repository; webhook deliveries fan out to each of them.
	ListApplicationsByRepo(ctx context.Context, provider, owner, repo string) ([]*Application, error)
	ListApplications(ctx context.Context) ([]*Application, error)
	// ListApplicationsForUser returns applications accessible to the user:
	// apps with no groups assigned (open to all) plus apps where the user is
	// a member of at least one assigned group.
	ListApplicationsForUser(ctx context.Context, userID uuid.UUID) ([]*Application, error)
	// CanUserAccessApplication returns true when the user may access the app:
	// either no groups are assigned (open to all) or the user belongs to one.
	CanUserAccessApplication(ctx context.Context, userID, appID uuid.UUID) (bool, error)
	UpdateApplication(ctx context.Context, a *Application) error
	DeleteApplication(ctx context.Context, id uuid.UUID) error
	// ListLatestRuns returns the most recent pipeline run per application,
	// keyed by application ID. Applications without runs are absent.
	ListLatestRuns(ctx context.Context) (map[uuid.UUID]*PipelineRun, error)

	// Commit lineage
	// AdvanceApplicationHead compare-and-swaps last_known_sha from `from` to
	// `to`. Returns false when the stored head no longer equals `from` (e.g. a
	// duplicate or out-of-order webhook delivery already advanced it) or the
	// application is blocked.
	AdvanceApplicationHead(ctx context.Context, id uuid.UUID, from, to string) (bool, error)
	// BlockApplication marks the application as requiring manual head
	// reconciliation (force push / history rewrite detected).
	BlockApplication(ctx context.Context, id uuid.UUID, reason string) error
	// AcceptApplicationHead re-baselines the tracked branch head to `head` and
	// unblocks the application.
	AcceptApplicationHead(ctx context.Context, id uuid.UUID, head string) error
	// CancelPendingRuns cancels all pending runs of an application and returns
	// their IDs. Used when the application is blocked: queued runs may point at
	// commits that no longer exist on the rewritten branch.
	CancelPendingRuns(ctx context.Context, applicationID uuid.UUID) ([]uuid.UUID, error)

	// Application group access
	GrantGroupAccess(ctx context.Context, applicationID, groupID uuid.UUID) error
	RevokeGroupAccess(ctx context.Context, applicationID, groupID uuid.UUID) error
	ListApplicationGroups(ctx context.Context, applicationID uuid.UUID) ([]*Group, error)

	// Dashboard
	GetDashboardStats(ctx context.Context) (*DashboardStats, error)

	// Pipeline runs
	// CreatePipelineRun inserts the run. Returns ErrDuplicateTriggerTag when a
	// run with the same non-empty TriggerTag already exists for the application.
	CreatePipelineRun(ctx context.Context, run *PipelineRun) error
	// GetRunByTriggerTag returns the run created for the given trigger tag, or
	// nil, nil when the tag has not triggered a run yet.
	GetRunByTriggerTag(ctx context.Context, applicationID uuid.UUID, tag string) (*PipelineRun, error)
	GetPipelineRun(ctx context.Context, id uuid.UUID) (*PipelineRun, error)
	ListPipelineRuns(ctx context.Context, applicationID uuid.UUID, limit, offset int, filter RunFilter) ([]*PipelineRun, error)
	UpdatePipelineRun(ctx context.Context, run *PipelineRun) error
	// ClaimPendingRun atomically selects the oldest claimable pending run, marks
	// it running owned by instanceID with a fresh lease, and returns it.
	// At most one run per application is running at any time, enforced across
	// instances. Returns nil, nil when there is nothing to claim.
	ClaimPendingRun(ctx context.Context, instanceID string) (*PipelineRun, error)
	// HeartbeatRun extends the lease of a running run owned by instanceID.
	// Returns false when the lease was lost: the run is no longer running
	// (e.g. cancelled via the API) or is owned by another instance.
	HeartbeatRun(ctx context.Context, id uuid.UUID, instanceID string) (bool, error)
	// ReapExpiredRuns resets running runs whose lease has expired back to
	// pending so any live instance can pick them up. Returns the number of
	// runs reset. Safe to call concurrently from multiple instances.
	ReapExpiredRuns(ctx context.Context) (int, error)
	// UpdateRunTag persists the version tag computed by the semver step so it
	// can be restored if the run is retried from a later step.
	UpdateRunTag(ctx context.Context, id uuid.UUID, tag string) error
	// MarkRunReleased sets released_at = NOW() for a run that completed all of
	// its steps successfully.
	MarkRunReleased(ctx context.Context, id uuid.UUID) error
	// GetLastReleasedRun returns the most recently released run for the
	// application on the given branch, or nil, nil if none exists yet.
	GetLastReleasedRun(ctx context.Context, applicationID uuid.UUID, branch string) (*PipelineRun, error)
	// ResetRunToPending resets a run back to pending (clearing its lease) so the
	// poller picks it up again. Used by step retries.
	ResetRunToPending(ctx context.Context, id uuid.UUID) error
	// CancelRun marks a pending or running run as cancelled. It is a no-op if the
	// run has already reached a terminal state.
	CancelRun(ctx context.Context, id uuid.UUID) error

	// Step results
	CreateStepResult(ctx context.Context, r *StepResult) error
	UpdateStepResult(ctx context.Context, r *StepResult) error
	ListStepResults(ctx context.Context, runID uuid.UUID) ([]*StepResult, error)
	DeleteStepResultsFrom(ctx context.Context, runID uuid.UUID, fromStepIndex int) error
	// ResetStepResultsFrom resets steps from fromStepIndex onwards back to
	// pending so they remain visible in the UI while the run is re-queued.
	// ExecuteFrom will delete and recreate them when it actually runs.
	ResetStepResultsFrom(ctx context.Context, runID uuid.UUID, fromStepIndex int) error
	GetStepResultByExternalRunID(ctx context.Context, externalRunID int64) (*StepResult, error)
	// SetStepResultExternalRunID persists the external workflow run ID for a
	// run+step immediately after dispatch, so an interrupted run can re-attach
	// to the already-dispatched workflow instead of dispatching a second time.
	SetStepResultExternalRunID(ctx context.Context, runID uuid.UUID, stepIndex int, externalRunID int64) error
	// OverrideStepResult marks a failed step as manually overridden so the run
	// can resume from the step after it. by and reason are recorded for the
	// audit trail. Returns false when the step is not in a failed state.
	OverrideStepResult(ctx context.Context, runID uuid.UUID, stepIndex int, by, reason string) (bool, error)

	// Users
	CreateUser(ctx context.Context, u *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserForAuth(ctx context.Context, email string) (*User, error) // includes PasswordHash
	ListUsers(ctx context.Context) ([]*User, error)
	CountAdmins(ctx context.Context) (int, error)
	UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	SetUserAdmin(ctx context.Context, id uuid.UUID, isAdmin bool) error
	DeleteUser(ctx context.Context, id uuid.UUID) error

	// Groups
	CreateGroup(ctx context.Context, g *Group) error
	GetGroup(ctx context.Context, id uuid.UUID) (*Group, error)
	ListGroups(ctx context.Context) ([]*Group, error)
	UpdateGroup(ctx context.Context, g *Group) error
	DeleteGroup(ctx context.Context, id uuid.UUID) error
	AddUserToGroup(ctx context.Context, userID, groupID uuid.UUID) error
	RemoveUserFromGroup(ctx context.Context, userID, groupID uuid.UUID) error
	ListGroupMembers(ctx context.Context, groupID uuid.UUID) ([]*User, error)

	// Approval requests
	CreateApprovalRequest(ctx context.Context, r *ApprovalRequest) error
	GetApprovalRequest(ctx context.Context, id uuid.UUID) (*ApprovalRequest, error)
	GetPendingApproval(ctx context.Context, runID uuid.UUID, stepIndex int) (*ApprovalRequest, error)
	// GetApprovalForStep returns the most recent approval request for a run+step
	// regardless of its status. Used on recovery to detect already-resolved approvals.
	GetApprovalForStep(ctx context.Context, runID uuid.UUID, stepIndex int) (*ApprovalRequest, error)
	// DeleteApprovalRequestsFrom removes all approval requests for a run from
	// stepIndex onwards. Used by RetryStep so a fresh approval is created.
	DeleteApprovalRequestsFrom(ctx context.Context, runID uuid.UUID, fromStepIndex int) error
	ResolveApprovalRequest(ctx context.Context, id uuid.UUID, status, resolvedBy string) error
	SupersedeOlderApprovals(ctx context.Context, applicationID uuid.UUID, stepIndex int, approvedRunID uuid.UUID) ([]uuid.UUID, error)
	ListApprovalRequests(ctx context.Context, runID uuid.UUID) ([]*ApprovalRequest, error)

	Close()
}
