package provider

import (
	"context"
	"errors"
	"net/http"
)

// ErrNotPushEvent is returned by ParseWebhook when the incoming webhook is not a push event.
var ErrNotPushEvent = errors.New("not a push event")

// ErrNotWorkflowRunEvent is returned by ParseWorkflowRun when the event is not a workflow_run.
var ErrNotWorkflowRunEvent = errors.New("not a workflow_run event")

// ErrNotFound is returned (wrapped) by lookups for objects that do not exist,
// e.g. GetTagCommitSHA for a tag that has not been created.
var ErrNotFound = errors.New("not found")

// PushEvent holds the normalised data extracted from a provider webhook push event.
type PushEvent struct {
	ProviderID string
	RepoOwner  string
	RepoName   string
	// Branch is the pushed branch name; empty for tag pushes.
	Branch string
	// TagName is set for tag pushes instead of Branch. For annotated tags
	// CommitSHA may be the tag object SHA; resolve via GetTagCommitSHA.
	TagName   string
	CommitSHA string
	// BeforeSHA is the branch head before this push (the payload's 'before'
	// field). For any non-force push it equals the head bifrost last saw; for a
	// merge commit it is the commit's first parent. All zeros when the branch
	// was just created.
	BeforeSHA string
	// Forced is true when the provider flags the push as a force push.
	// Not all providers send this; absence of the flag is not proof of a
	// fast-forward push (ancestry must be checked when BeforeSHA mismatches).
	Forced      bool
	CommitMsg   string
	AuthorName  string
	AuthorEmail string
	// ChangedFiles is the union of added and modified files across all commits in the push.
	ChangedFiles []string
}

// ZeroSHA is the all-zeros SHA providers send as 'before' when a branch is
// created (and as 'after' when it is deleted).
const ZeroSHA = "0000000000000000000000000000000000000000"

// CompareStatus describes how a head commit relates to a base commit.
type CompareStatus string

const (
	// CompareAhead: base is an ancestor of head (fast-forward).
	CompareAhead CompareStatus = "ahead"
	// CompareBehind: head is an ancestor of base (stale delivery).
	CompareBehind CompareStatus = "behind"
	// CompareIdentical: base and head are the same commit.
	CompareIdentical CompareStatus = "identical"
	// CompareDiverged: neither is an ancestor of the other (history rewrite).
	CompareDiverged CompareStatus = "diverged"
)

// WorkflowRun represents the current state of a remote CI/CD workflow run.
type WorkflowRun struct {
	ID         int64
	Status     string // "queued", "in_progress", "completed"
	Conclusion string // "success", "failure", "cancelled", "skipped", ""
}

// WorkflowRunEvent is the normalised payload from a provider's workflow_run webhook.
type WorkflowRunEvent struct {
	RunID      int64
	Action     string // "requested", "in_progress", "completed"
	Status     string // "queued", "in_progress", "completed"
	Conclusion string // "success", "failure", "cancelled", "skipped", ""
	Name       string // workflow name
}

// Commit is a minimal view of a repository commit used by changelog generation.
type Commit struct {
	SHA     string
	Message string
	Author  string
}

// Provider is the common interface that every VCS/CI provider must satisfy.
type Provider interface {
	// ID returns a stable, unique identifier for this provider (e.g. "github").
	ID() string

	// ParseWebhook validates and decodes an incoming webhook request.
	// Returns ErrNotPushEvent when the event type is not a push.
	ParseWebhook(r *http.Request, secret string) (PushEvent, error)

	// ListTags returns all tag names for the given repository.
	ListTags(ctx context.Context, owner, repo string) ([]string, error)

	// CreateTag creates an annotated tag pointing at sha.
	CreateTag(ctx context.Context, owner, repo, tag, sha, message string) error

	// GetTagCommitSHA returns the commit SHA a tag ultimately points at,
	// resolving annotated tag objects to their target commit.
	// Returns an error wrapping ErrNotFound when the tag does not exist.
	// Used to make tag creation idempotent across pipeline restarts.
	GetTagCommitSHA(ctx context.Context, owner, repo, tag string) (string, error)

	// GetReleaseByTag returns the HTML URL of an existing release for tag.
	// Returns an error wrapping ErrNotFound when no release exists.
	// Used to make release creation idempotent across pipeline restarts.
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (string, error)

	// DispatchWorkflow triggers a workflow file by name on the given ref.
	// Returns the numeric workflow run ID and its HTML URL.
	// When the run ID cannot be determined (e.g. the provider does not expose it
	// synchronously), id is 0 and url is empty.
	DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) (id int64, url string, err error)

	// GetWorkflowRun fetches the current state of a workflow run by its numeric ID.
	GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (WorkflowRun, error)

	// ParseWorkflowRun decodes a validated webhook payload as a workflow_run event.
	// Returns ErrNotWorkflowRunEvent if the event type is not "workflow_run".
	ParseWorkflowRun(eventType string, payload []byte) (WorkflowRunEvent, error)

	// ListCommitsSince returns commits reachable from head but not from base (exclusive).
	// base is typically a tag name (e.g. "v0.1.5"), head is a commit SHA.
	// Returns an empty slice (not error) when base is empty (no prior release).
	ListCommitsSince(ctx context.Context, owner, repo, base, head string) ([]Commit, error)

	// CreateRelease creates a VCS release and returns its HTML URL.
	CreateRelease(ctx context.Context, owner, repo, tag, name, body string, draft, prerelease bool) (string, error)

	// CompareCommits reports how head relates to base in the repository.
	// Returns an error wrapping ErrNotFound when either commit is unknown to
	// the repository (e.g. orphaned by a force push).
	CompareCommits(ctx context.Context, owner, repo, base, head string) (CompareStatus, error)

	// GetBranchHead returns the current head commit SHA of the branch.
	GetBranchHead(ctx context.Context, owner, repo, branch string) (string, error)

	// InstallWebhook creates or updates a webhook on the repository pointing at webhookURL.
	// If a webhook for webhookURL already exists it is updated in-place; otherwise a new one is created.
	InstallWebhook(ctx context.Context, owner, repo, webhookURL, secret string, events []string) error
}

// CommitFilesLister is implemented by providers that can list the files
// changed by a single commit. The changelog step uses it to scope entries to
// an application's paths; providers without it skip path filtering.
type CommitFilesLister interface {
	ListCommitFiles(ctx context.Context, owner, repo, sha string) ([]string, error)
}

// ReleaseNotesGenerator is implemented by providers whose API can auto-generate
// release notes between two tags (currently GitHub only). ChangelogStep uses it
// when available; providers without it (Gitea/Forgejo) fall back to manually
// grouping commits via ListCommitsSince.
type ReleaseNotesGenerator interface {
	// GenerateReleaseNotes returns formatted release notes for tag, diffed against
	// previousTag. targetCommitish is the commit the tag would point at if it does
	// not exist yet.
	GenerateReleaseNotes(ctx context.Context, owner, repo, tag, previousTag, targetCommitish string) (string, error)
}
