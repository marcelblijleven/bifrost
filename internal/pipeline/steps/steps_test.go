package steps_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/pipeline/steps"
	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func init() {
	// Speed up polling so tests run in milliseconds rather than seconds.
	steps.SetApprovalPollInterval(10 * time.Millisecond)
	steps.SetDispatchPollInterval(10 * time.Millisecond)
}

// stepStore implements store.Store with only the approval methods wired up.
// Calling any other method panics, which flags unexpected store usage in tests.
type stepStore struct {
	store.Store // nil — panics on any method not overridden below

	createApproval     func(context.Context, *store.ApprovalRequest) error
	getApproval        func(context.Context, uuid.UUID) (*store.ApprovalRequest, error)
	getLastReleasedRun func(context.Context, uuid.UUID, string) (*store.PipelineRun, error)

	// persistedExternalRunIDs records SetStepResultExternalRunID calls, keyed
	// by step index.
	persistedExternalRunIDs map[int]int64
}

func (m *stepStore) SetStepResultExternalRunID(_ context.Context, _ uuid.UUID, stepIndex int, externalRunID int64) error {
	if m.persistedExternalRunIDs == nil {
		m.persistedExternalRunIDs = make(map[int]int64)
	}
	m.persistedExternalRunIDs[stepIndex] = externalRunID
	return nil
}

func (m *stepStore) GetLastReleasedRun(ctx context.Context, applicationID uuid.UUID, branch string) (*store.PipelineRun, error) {
	if m.getLastReleasedRun != nil {
		return m.getLastReleasedRun(ctx, applicationID, branch)
	}
	return nil, nil
}

func (m *stepStore) CreateApprovalRequest(ctx context.Context, r *store.ApprovalRequest) error {
	if m.createApproval != nil {
		return m.createApproval(ctx, r)
	}
	return nil
}

func (m *stepStore) GetApprovalRequest(ctx context.Context, id uuid.UUID) (*store.ApprovalRequest, error) {
	if m.getApproval != nil {
		return m.getApproval(ctx, id)
	}
	return &store.ApprovalRequest{Status: "pending"}, nil
}

// GetApprovalForStep returns not-found so tests always create a fresh approval request,
// simulating a clean (non-recovery) run.
func (m *stepStore) GetApprovalForStep(_ context.Context, _ uuid.UUID, _ int) (*store.ApprovalRequest, error) {
	return nil, errors.New("not found")
}

func (m *stepStore) UpdateRunTag(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (m *stepStore) GetPipelineRun(_ context.Context, _ uuid.UUID) (*store.PipelineRun, error) {
	return nil, errors.New("not found")
}

// stepProvider implements provider.Provider with only Dispatch and GetWorkflowRun wired up.
type stepProvider struct {
	provider.Provider // nil — panics on any method not overridden below

	dispatch         func(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) (int64, string, error)
	getWorkflowRun   func(ctx context.Context, owner, repo string, runID int64) (provider.WorkflowRun, error)
	listCommitsSince func(ctx context.Context, owner, repo, base, head string) ([]provider.Commit, error)
	listTags         func(ctx context.Context, owner, repo string) ([]string, error)
	createTag        func(ctx context.Context, owner, repo, tag, sha, message string) error
	getTagCommitSHA  func(ctx context.Context, owner, repo, tag string) (string, error)
	createRelease    func(ctx context.Context, owner, repo, tag, name, body string, draft, prerelease bool) (string, error)
	getReleaseByTag  func(ctx context.Context, owner, repo, tag string) (string, error)
}

func (p *stepProvider) ID() string { return "test" }

func (p *stepProvider) ParseWebhook(_ *http.Request, _ string) (provider.PushEvent, error) {
	panic("unexpected call to ParseWebhook")
}

func (p *stepProvider) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) (int64, string, error) {
	if p.dispatch != nil {
		return p.dispatch(ctx, owner, repo, workflow, ref, inputs)
	}
	return 1, "https://example.com/actions/runs/1", nil
}

func (p *stepProvider) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (provider.WorkflowRun, error) {
	if p.getWorkflowRun != nil {
		return p.getWorkflowRun(ctx, owner, repo, runID)
	}
	return provider.WorkflowRun{Status: "completed", Conclusion: "success"}, nil
}

func (p *stepProvider) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	if p.listTags != nil {
		return p.listTags(ctx, owner, repo)
	}
	return []string{}, nil
}
func (p *stepProvider) CreateTag(ctx context.Context, owner, repo, tag, sha, message string) error {
	if p.createTag != nil {
		return p.createTag(ctx, owner, repo, tag, sha, message)
	}
	return nil
}
func (p *stepProvider) GetTagCommitSHA(ctx context.Context, owner, repo, tag string) (string, error) {
	if p.getTagCommitSHA != nil {
		return p.getTagCommitSHA(ctx, owner, repo, tag)
	}
	return "", provider.ErrNotFound
}
func (p *stepProvider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (string, error) {
	if p.getReleaseByTag != nil {
		return p.getReleaseByTag(ctx, owner, repo, tag)
	}
	return "", provider.ErrNotFound
}
func (p *stepProvider) ListCommitsSince(ctx context.Context, owner, repo, base, head string) ([]provider.Commit, error) {
	if p.listCommitsSince != nil {
		return p.listCommitsSince(ctx, owner, repo, base, head)
	}
	return nil, nil
}
func (p *stepProvider) CreateRelease(ctx context.Context, owner, repo, tag, name, body string, draft, prerelease bool) (string, error) {
	if p.createRelease != nil {
		return p.createRelease(ctx, owner, repo, tag, name, body, draft, prerelease)
	}
	return "https://github.com/example/releases/tag/v0.0.1", nil
}
func (p *stepProvider) ParseWorkflowRun(_ string, _ []byte) (provider.WorkflowRunEvent, error) {
	return provider.WorkflowRunEvent{}, nil
}

func newSC(st store.Store, prov provider.Provider) *pipeline.StepContext {
	return &pipeline.StepContext{
		RunID:    uuid.New(),
		Store:    st,
		Provider: prov,
		Event: provider.PushEvent{
			RepoOwner: "owner",
			RepoName:  "repo",
			Branch:    "main",
		},
	}
}

// approvalStore returns a store whose GetApprovalRequest transitions through the
// provided statuses in order (repeating the last one indefinitely).
func approvalStore(statuses ...string) *stepStore {
	var calls atomic.Int32
	return &stepStore{
		getApproval: func(_ context.Context, _ uuid.UUID) (*store.ApprovalRequest, error) {
			i := int(calls.Add(1)) - 1
			if i >= len(statuses) {
				i = len(statuses) - 1
			}
			return &store.ApprovalRequest{Status: statuses[i], ResolvedBy: "alice"}, nil
		},
	}
}

// ── NewDispatchStep ───────────────────────────────────────────────────────────

func TestNewDispatchStep_MissingWorkflow(t *testing.T) {
	_, err := steps.NewDispatchStep(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing workflow, got nil")
	}
}

func TestNewDispatchStep_Defaults(t *testing.T) {
	s, err := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name() != "dispatch_workflow:deploy.yml" {
		t.Errorf("Name() = %q, want %q", s.Name(), "dispatch_workflow:deploy.yml")
	}
}

func TestNewDispatchStep_AllOptions(t *testing.T) {
	s, err := steps.NewDispatchStep(map[string]any{
		"workflow":               "deploy.yml",
		"wait":                   true,
		"timeout_minutes":        float64(45),
		"require_approval":       true,
		"approval_message":       "Ship it?",
		"approval_timeout_hours": float64(8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name() != "dispatch_workflow:deploy.yml" {
		t.Errorf("Name() = %q", s.Name())
	}
}

// ── DispatchStep.Run — no approval ───────────────────────────────────────────

func TestDispatchStep_NoWait_Success(t *testing.T) {
	dispatched := false
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, workflow, _ string, _ map[string]string) (int64, string, error) {
			dispatched = true
			if workflow != "deploy.yml" {
				t.Errorf("dispatched wrong workflow: %q", workflow)
			}
			return 42, "", nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml"})
	sc := newSC(&stepStore{}, prov)

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !dispatched {
		t.Error("DispatchWorkflow was not called")
	}
	if sc.ExternalRunID == nil || *sc.ExternalRunID != 42 {
		t.Errorf("ExternalRunID = %v, want 42", sc.ExternalRunID)
	}
}

func TestDispatchStep_NoWait_UsesBranchWhenNoTag(t *testing.T) {
	var usedRef string
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, ref string, _ map[string]string) (int64, string, error) {
			usedRef = ref
			return 1, "", nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml"})
	sc := newSC(&stepStore{}, prov)
	sc.Event.Branch = "main"
	sc.Tag = "" // no tag set

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatal(err)
	}
	if usedRef != "main" {
		t.Errorf("ref = %q, want %q", usedRef, "main")
	}
}

func TestDispatchStep_NoWait_UseTagAsRef(t *testing.T) {
	var usedRef string
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, ref string, _ map[string]string) (int64, string, error) {
			usedRef = ref
			return 1, "", nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml"})
	sc := newSC(&stepStore{}, prov)
	sc.Tag = "v1.2.3"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatal(err)
	}
	if usedRef != "v1.2.3" {
		t.Errorf("ref = %q, want %q", usedRef, "v1.2.3")
	}
}

func TestDispatchStep_DispatchError(t *testing.T) {
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 0, "", fmt.Errorf("github api down")
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml"})
	err := s.Run(t.Context(), newSC(&stepStore{}, prov))
	if err == nil || !errors.Is(err, fmt.Errorf("github api down")) {
		// just check non-nil; the error is wrapped
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	}
}

// ── DispatchStep.Run — wait=true ─────────────────────────────────────────────

func TestDispatchStep_Wait_Success(t *testing.T) {
	var polls atomic.Int32
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 99, "", nil
		},
		getWorkflowRun: func(_ context.Context, _, _ string, runID int64) (provider.WorkflowRun, error) {
			if runID != 99 {
				t.Errorf("GetWorkflowRun called with wrong runID %d", runID)
			}
			n := polls.Add(1)
			if n < 3 {
				return provider.WorkflowRun{Status: "in_progress"}, nil
			}
			return provider.WorkflowRun{Status: "completed", Conclusion: "success"}, nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml", "wait": true})
	if err := s.Run(t.Context(), newSC(&stepStore{}, prov)); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if polls.Load() < 3 {
		t.Errorf("expected at least 3 polls, got %d", polls.Load())
	}
}

func TestDispatchStep_Wait_Failure(t *testing.T) {
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 1, "", nil
		},
		getWorkflowRun: func(_ context.Context, _, _ string, _ int64) (provider.WorkflowRun, error) {
			return provider.WorkflowRun{Status: "completed", Conclusion: "failure"}, nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml", "wait": true})
	err := s.Run(t.Context(), newSC(&stepStore{}, prov))
	if err == nil {
		t.Fatal("expected error for failed workflow, got nil")
	}
}

func TestDispatchStep_Wait_ContextCancelled(t *testing.T) {
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 1, "", nil
		},
		getWorkflowRun: func(_ context.Context, _, _ string, _ int64) (provider.WorkflowRun, error) {
			return provider.WorkflowRun{Status: "in_progress"}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml", "wait": true})
	err := s.Run(ctx, newSC(&stepStore{}, prov))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ── DispatchStep.Run — require_approval ──────────────────────────────────────

func TestDispatchStep_RequireApproval_Approved(t *testing.T) {
	dispatched := false
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			dispatched = true
			return 1, "", nil
		},
	}
	st := approvalStore("pending", "approved")

	s, _ := steps.NewDispatchStep(map[string]any{
		"workflow":         "deploy.yml",
		"require_approval": true,
	})
	if err := s.Run(t.Context(), newSC(st, prov)); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !dispatched {
		t.Error("DispatchWorkflow was not called after approval")
	}
}

func TestDispatchStep_RequireApproval_DefaultMessage(t *testing.T) {
	var gotMessage string
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 1, "", nil
		},
	}
	st := &stepStore{
		createApproval: func(_ context.Context, r *store.ApprovalRequest) error {
			gotMessage = r.Message
			return nil
		},
		getApproval: func(_ context.Context, _ uuid.UUID) (*store.ApprovalRequest, error) {
			return &store.ApprovalRequest{Status: "approved"}, nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{
		"workflow":         "deploy.yml",
		"require_approval": true,
	})
	s.Run(t.Context(), newSC(st, prov)) //nolint:errcheck

	want := `Approve dispatch of workflow "deploy.yml"?`
	if gotMessage != want {
		t.Errorf("approval message = %q, want %q", gotMessage, want)
	}
}

func TestDispatchStep_RequireApproval_CustomMessage(t *testing.T) {
	var gotMessage string
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 1, "", nil
		},
	}
	st := &stepStore{
		createApproval: func(_ context.Context, r *store.ApprovalRequest) error {
			gotMessage = r.Message
			return nil
		},
		getApproval: func(_ context.Context, _ uuid.UUID) (*store.ApprovalRequest, error) {
			return &store.ApprovalRequest{Status: "approved"}, nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{
		"workflow":         "deploy.yml",
		"require_approval": true,
		"approval_message": "Ready to ship?",
	})
	s.Run(t.Context(), newSC(st, prov)) //nolint:errcheck

	if gotMessage != "Ready to ship?" {
		t.Errorf("approval message = %q, want %q", gotMessage, "Ready to ship?")
	}
}

func TestDispatchStep_RequireApproval_Rejected(t *testing.T) {
	dispatched := false
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			dispatched = true
			return 1, "", nil
		},
	}
	st := approvalStore("rejected")

	s, _ := steps.NewDispatchStep(map[string]any{
		"workflow":         "deploy.yml",
		"require_approval": true,
	})
	err := s.Run(t.Context(), newSC(st, prov))
	if err == nil {
		t.Fatal("expected error on rejection, got nil")
	}
	if dispatched {
		t.Error("DispatchWorkflow must not be called after rejection")
	}
}

func TestDispatchStep_RequireApproval_Superseded(t *testing.T) {
	dispatched := false
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			dispatched = true
			return 1, "", nil
		},
	}
	st := approvalStore("superseded")

	s, _ := steps.NewDispatchStep(map[string]any{
		"workflow":         "deploy.yml",
		"require_approval": true,
	})
	err := s.Run(t.Context(), newSC(st, prov))
	if !errors.Is(err, pipeline.ErrSuperseded) {
		t.Errorf("expected ErrSuperseded, got %v", err)
	}
	if dispatched {
		t.Error("DispatchWorkflow must not be called after supersession")
	}
}

func TestDispatchStep_RequireApproval_ContextCancelledDuringWait(t *testing.T) {
	dispatched := false
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			dispatched = true
			return 1, "", nil
		},
	}
	st := approvalStore("pending") // never resolves

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)

	s, _ := steps.NewDispatchStep(map[string]any{
		"workflow":         "deploy.yml",
		"require_approval": true,
	})
	err := s.Run(ctx, newSC(st, prov))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if dispatched {
		t.Error("DispatchWorkflow must not be called when context is cancelled")
	}
}

// ── ApprovalStep ──────────────────────────────────────────────────────────────

func TestApprovalStep_Approved(t *testing.T) {
	s, _ := steps.NewApprovalStep(map[string]any{"message": "ok?"})
	err := s.Run(t.Context(), newSC(approvalStore("pending", "approved"), &stepProvider{}))
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestApprovalStep_Rejected(t *testing.T) {
	s, _ := steps.NewApprovalStep(map[string]any{})
	err := s.Run(t.Context(), newSC(approvalStore("rejected"), &stepProvider{}))
	if err == nil {
		t.Fatal("expected error on rejection")
	}
}

func TestApprovalStep_Superseded(t *testing.T) {
	s, _ := steps.NewApprovalStep(map[string]any{})
	err := s.Run(t.Context(), newSC(approvalStore("superseded"), &stepProvider{}))
	if !errors.Is(err, pipeline.ErrSuperseded) {
		t.Errorf("expected ErrSuperseded, got %v", err)
	}
}

func TestApprovalStep_ContextCancelled(t *testing.T) {
	s, _ := steps.NewApprovalStep(map[string]any{})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)

	err := s.Run(ctx, newSC(approvalStore("pending"), &stepProvider{}))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestApprovalStep_CreateApprovalError(t *testing.T) {
	st := &stepStore{
		createApproval: func(_ context.Context, _ *store.ApprovalRequest) error {
			return fmt.Errorf("db down")
		},
	}
	s, _ := steps.NewApprovalStep(map[string]any{})
	err := s.Run(t.Context(), newSC(st, &stepProvider{}))
	if err == nil {
		t.Fatal("expected error when CreateApprovalRequest fails")
	}
}

func TestApprovalStep_StepIndexIsRecorded(t *testing.T) {
	var got int
	st := &stepStore{
		createApproval: func(_ context.Context, r *store.ApprovalRequest) error {
			got = r.StepIndex
			return nil
		},
		getApproval: func(_ context.Context, _ uuid.UUID) (*store.ApprovalRequest, error) {
			return &store.ApprovalRequest{Status: "approved"}, nil
		},
	}

	s, _ := steps.NewApprovalStep(map[string]any{})
	sc := newSC(st, &stepProvider{})
	sc.StepIndex = 3

	s.Run(t.Context(), sc) //nolint:errcheck

	if got != 3 {
		t.Errorf("StepIndex in approval request = %d, want 3", got)
	}
}

func TestApprovalStep_NotifiesOnApprovalURL(t *testing.T) {
	received := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := approvalStore("approved")
	s, _ := steps.NewApprovalStep(map[string]any{"message": "please look"})
	sc := newSC(st, &stepProvider{})
	sc.Application.Notifications.OnApprovalURL = srv.URL

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case body := <-received:
		if body["event"] != "pipeline.approval_requested" {
			t.Errorf("event = %v, want pipeline.approval_requested", body["event"])
		}
		if body["message"] != "please look" {
			t.Errorf("message = %v, want %q", body["message"], "please look")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval notification")
	}
}

func TestApprovalStep_NoNotificationWhenURLUnset(t *testing.T) {
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := approvalStore("approved")
	s, _ := steps.NewApprovalStep(map[string]any{})
	sc := newSC(st, &stepProvider{})

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if called.Load() {
		t.Error("notification was sent despite no OnApprovalURL configured")
	}
}

// ── SemverStep ────────────────────────────────────────────────────────────────

func TestSemverStep_ScansAllCommitsSinceLastTag(t *testing.T) {
	st := &stepStore{}
	prov := &stepProvider{
		listTags: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{"v1.2.3"}, nil
		},
		listCommitsSince: func(_ context.Context, _, _, base, head string) ([]provider.Commit, error) {
			if base != "v1.2.3" || head != "sha-new" {
				t.Errorf("ListCommitsSince(base=%q, head=%q), want base=v1.2.3 head=sha-new", base, head)
			}
			return []provider.Commit{
				{Message: "feat: add thing"},
				{Message: "fix: typo"},
			}, nil
		},
	}

	s, _ := steps.NewSemverStep(map[string]any{})
	sc := newSC(st, prov)
	sc.Event.CommitSHA = "sha-new"
	sc.Event.CommitMsg = "fix: typo" // head commit alone would only bump patch

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if sc.Tag != "v1.3.0" {
		t.Errorf("Tag = %q, want %q (minor bump from feat commit earlier in the range)", sc.Tag, "v1.3.0")
	}
}

func TestSemverStep_FallsBackToHeadCommitOnListCommitsError(t *testing.T) {
	st := &stepStore{}
	prov := &stepProvider{
		listTags: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{"v1.0.0"}, nil
		},
		listCommitsSince: func(_ context.Context, _, _, _, _ string) ([]provider.Commit, error) {
			return nil, errors.New("boom")
		},
	}

	s, _ := steps.NewSemverStep(map[string]any{})
	sc := newSC(st, prov)
	sc.Event.CommitMsg = "feat: new thing"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if sc.Tag != "v1.1.0" {
		t.Errorf("Tag = %q, want %q", sc.Tag, "v1.1.0")
	}
}

func TestSemverStep_TagPrefix_NamespacesTags(t *testing.T) {
	st := &stepStore{}
	var gotBase string
	prov := &stepProvider{
		listTags: func(_ context.Context, _, _ string) ([]string, error) {
			// Monorepo: several applications tag the same repository.
			return []string{"frontend-v1.2.0", "backend-v9.0.0", "v3.0.0"}, nil
		},
		listCommitsSince: func(_ context.Context, _, _, base, _ string) ([]provider.Commit, error) {
			gotBase = base
			return []provider.Commit{{Message: "fix: bug"}}, nil
		},
	}

	s, _ := steps.NewSemverStep(map[string]any{})
	sc := newSC(st, prov)
	sc.Application.TagPrefix = "frontend-"
	sc.Event.CommitSHA = "sha-new"
	sc.Event.CommitMsg = "fix: bug"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	// Other applications' tags (backend-v9.0.0, v3.0.0) must not leak into
	// this application's version sequence.
	if sc.Tag != "frontend-v1.2.1" {
		t.Errorf("Tag = %q, want frontend-v1.2.1", sc.Tag)
	}
	if gotBase != "frontend-v1.2.0" {
		t.Errorf("ListCommitsSince base = %q, want the full prefixed tag frontend-v1.2.0", gotBase)
	}
}

func TestSemverStep_TagPrefix_FirstRelease(t *testing.T) {
	st := &stepStore{}
	prov := &stepProvider{
		listTags: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{"backend-v9.0.0"}, nil // no tags for this prefix yet
		},
	}

	s, _ := steps.NewSemverStep(map[string]any{})
	sc := newSC(st, prov)
	sc.Application.TagPrefix = "frontend-"
	sc.Event.CommitMsg = "feat: initial"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if sc.Tag != "frontend-v0.1.0" {
		t.Errorf("Tag = %q, want frontend-v0.1.0", sc.Tag)
	}
}

// ── ChangelogStep ─────────────────────────────────────────────────────────────

// genNotesProvider adds provider.ReleaseNotesGenerator to stepProvider so tests
// can simulate a GitHub-like provider.
type genNotesProvider struct {
	*stepProvider
	generateReleaseNotes func(ctx context.Context, owner, repo, tag, previousTag, targetCommitish string) (string, error)
}

func (p *genNotesProvider) GenerateReleaseNotes(ctx context.Context, owner, repo, tag, previousTag, targetCommitish string) (string, error) {
	return p.generateReleaseNotes(ctx, owner, repo, tag, previousTag, targetCommitish)
}

// commitFilesProvider adds provider.CommitFilesLister to stepProvider so tests
// can exercise path-scoped changelogs.
type commitFilesProvider struct {
	*stepProvider
	files map[string][]string // sha → changed files; missing sha → error
}

func (p *commitFilesProvider) ListCommitFiles(_ context.Context, _, _, sha string) ([]string, error) {
	f, ok := p.files[sha]
	if !ok {
		return nil, errors.New("unknown commit")
	}
	return f, nil
}

func TestChangelogStep_PathFilter_ScopesCommitsToApplication(t *testing.T) {
	st := &stepStore{
		getLastReleasedRun: func(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
			return &store.PipelineRun{Tag: "frontend-v1.0.0"}, nil
		},
	}
	prov := &commitFilesProvider{
		stepProvider: &stepProvider{
			listCommitsSince: func(_ context.Context, _, _, _, _ string) ([]provider.Commit, error) {
				return []provider.Commit{
					{SHA: "a", Message: "feat: frontend button"},
					{SHA: "b", Message: "feat: backend endpoint"},
					{SHA: "c", Message: "fix: mystery"},
				}, nil
			},
		},
		files: map[string][]string{
			"a": {"frontend/src/button.ts"},
			"b": {"backend/api/endpoint.go"},
			// "c" missing: the file lookup fails and the commit must be kept.
		},
	}

	s := &steps.ChangelogStep{}
	sc := newSC(st, prov)
	sc.Tag = "frontend-v1.1.0"
	sc.Event.CommitSHA = "sha-head"
	sc.Application.SkipConditions.PathsInclude = []string{"frontend/**"}

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !strings.Contains(sc.Changelog, "frontend button") {
		t.Errorf("Changelog must contain the frontend commit, got: %q", sc.Changelog)
	}
	if strings.Contains(sc.Changelog, "backend endpoint") {
		t.Errorf("Changelog must not contain other applications' commits, got: %q", sc.Changelog)
	}
	if !strings.Contains(sc.Changelog, "mystery") {
		t.Errorf("Commits whose files cannot be listed must be kept, got: %q", sc.Changelog)
	}
}

func TestChangelogStep_PathFilter_SkipsGenerateNotes(t *testing.T) {
	// GitHub's generate-notes API covers the whole repository; with path
	// filters the manual, filterable listing must be used instead.
	st := &stepStore{
		getLastReleasedRun: func(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
			return &store.PipelineRun{Tag: "v1.0.0"}, nil
		},
	}
	prov := &genNotesProvider{
		stepProvider: &stepProvider{
			listCommitsSince: func(_ context.Context, _, _, _, _ string) ([]provider.Commit, error) {
				return []provider.Commit{{SHA: "a", Message: "feat: thing"}}, nil
			},
		},
		generateReleaseNotes: func(_ context.Context, _, _, _, _, _ string) (string, error) {
			t.Error("generate-notes must not be used when path filters are set")
			return "", nil
		},
	}

	s := &steps.ChangelogStep{}
	sc := newSC(st, prov)
	sc.Tag = "v1.1.0"
	sc.Event.CommitSHA = "sha-head"
	sc.Application.SkipConditions.PathsInclude = []string{"frontend/**"}

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	// genNotesProvider is not a CommitFilesLister, so filtering is skipped and
	// all commits are kept — but via the manual path.
	if !strings.Contains(sc.Changelog, "feat: thing") {
		t.Errorf("Changelog = %q, want the manually listed commit", sc.Changelog)
	}
}

func TestChangelogStep_NoPriorRelease_ManualListing(t *testing.T) {
	st := &stepStore{
		getLastReleasedRun: func(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
			return nil, nil
		},
	}
	var gotBase string
	prov := &stepProvider{
		listCommitsSince: func(_ context.Context, _, _, base, _ string) ([]provider.Commit, error) {
			gotBase = base
			return []provider.Commit{{Message: "feat: thing"}}, nil
		},
	}

	s := &steps.ChangelogStep{}
	sc := newSC(st, prov)
	sc.Tag = "v1.0.0"
	sc.Event.CommitSHA = "sha1"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if gotBase != "" {
		t.Errorf("base = %q, want empty (no prior release)", gotBase)
	}
	if !strings.Contains(sc.Changelog, "feat: thing") {
		t.Errorf("Changelog = %q, want it to contain the commit message", sc.Changelog)
	}
}

func TestChangelogStep_GitHubGenerateNotes_UsedWhenPriorTagExists(t *testing.T) {
	st := &stepStore{
		getLastReleasedRun: func(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
			return &store.PipelineRun{Tag: "v1.0.0", CommitSHA: "old-sha"}, nil
		},
	}
	var gotTag, gotPrev, gotTarget string
	prov := &genNotesProvider{
		stepProvider: &stepProvider{
			listCommitsSince: func(_ context.Context, _, _, _, _ string) ([]provider.Commit, error) {
				t.Error("manual ListCommitsSince should not be called when generate-notes succeeds")
				return nil, nil
			},
		},
		generateReleaseNotes: func(_ context.Context, _, _, tag, previousTag, targetCommitish string) (string, error) {
			gotTag, gotPrev, gotTarget = tag, previousTag, targetCommitish
			return "## generated notes", nil
		},
	}

	s := &steps.ChangelogStep{}
	sc := newSC(st, prov)
	sc.Tag = "v1.1.0"
	sc.Event.CommitSHA = "new-sha"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if sc.Changelog != "## generated notes" {
		t.Errorf("Changelog = %q, want the generated notes body", sc.Changelog)
	}
	if gotTag != "v1.1.0" || gotPrev != "v1.0.0" || gotTarget != "new-sha" {
		t.Errorf("GenerateReleaseNotes called with (%q,%q,%q), want (v1.1.0, v1.0.0, new-sha)", gotTag, gotPrev, gotTarget)
	}
}

func TestChangelogStep_GitHubFallsBackWhenNoPriorTag(t *testing.T) {
	st := &stepStore{
		getLastReleasedRun: func(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
			return &store.PipelineRun{CommitSHA: "old-sha"}, nil // no Tag
		},
	}
	called := false
	var gotBase string
	prov := &genNotesProvider{
		stepProvider: &stepProvider{
			listCommitsSince: func(_ context.Context, _, _, base, _ string) ([]provider.Commit, error) {
				called = true
				gotBase = base
				return []provider.Commit{{Message: "fix: x"}}, nil
			},
		},
		generateReleaseNotes: func(context.Context, string, string, string, string, string) (string, error) {
			t.Error("generate-notes API should not be called without a prior tag")
			return "", nil
		},
	}

	s := &steps.ChangelogStep{}
	sc := newSC(st, prov)
	sc.Tag = "v1.1.0"
	sc.Event.CommitSHA = "new-sha"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected manual ListCommitsSince to be called")
	}
	if gotBase != "old-sha" {
		t.Errorf("base = %q, want %q (last released run's commit SHA)", gotBase, "old-sha")
	}
}

func TestChangelogStep_NonGitHubProvider_AlwaysManual(t *testing.T) {
	st := &stepStore{
		getLastReleasedRun: func(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
			return &store.PipelineRun{Tag: "v1.0.0"}, nil
		},
	}
	called := false
	prov := &stepProvider{
		listCommitsSince: func(_ context.Context, _, _, _, _ string) ([]provider.Commit, error) {
			called = true
			return []provider.Commit{{Message: "fix: x"}}, nil
		},
	}

	s := &steps.ChangelogStep{}
	sc := newSC(st, prov)
	sc.Tag = "v1.1.0"

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !called {
		t.Error("expected manual listing for a provider without ReleaseNotesGenerator")
	}
}

// ── Idempotent resume (recovery after interruption) ───────────────────────────

func TestDispatchStep_PersistsExternalRunIDImmediately(t *testing.T) {
	st := &stepStore{}
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			return 42, "", nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml"})
	sc := newSC(st, prov)
	sc.StepIndex = 2

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if got := st.persistedExternalRunIDs[2]; got != 42 {
		t.Errorf("persisted external run id = %d, want 42 (must be stored before the step completes)", got)
	}
}

func TestDispatchStep_ReattachesToPriorRun(t *testing.T) {
	var polledRunID int64
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			t.Error("DispatchWorkflow called; interrupted run must re-attach, not dispatch again")
			return 0, "", nil
		},
		getWorkflowRun: func(_ context.Context, _, _ string, runID int64) (provider.WorkflowRun, error) {
			polledRunID = runID
			return provider.WorkflowRun{ID: runID, Status: "completed", Conclusion: "success"}, nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml", "wait": true})
	sc := newSC(&stepStore{}, prov)
	sc.StepIndex = 1
	sc.PriorExternalRunIDs = map[int]int64{1: 42}

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if polledRunID != 42 {
		t.Errorf("polled workflow run %d, want the prior run 42", polledRunID)
	}
	if sc.ExternalRunID == nil || *sc.ExternalRunID != 42 {
		t.Errorf("ExternalRunID = %v, want 42", sc.ExternalRunID)
	}
}

func TestDispatchStep_Reattach_SkipsApproval(t *testing.T) {
	st := &stepStore{
		createApproval: func(_ context.Context, _ *store.ApprovalRequest) error {
			t.Error("approval requested; re-attach must not re-run the approval gate")
			return nil
		},
	}
	prov := &stepProvider{
		dispatch: func(_ context.Context, _, _, _, _ string, _ map[string]string) (int64, string, error) {
			t.Error("DispatchWorkflow called on re-attach")
			return 0, "", nil
		},
	}

	s, _ := steps.NewDispatchStep(map[string]any{"workflow": "deploy.yml", "require_approval": true})
	sc := newSC(st, prov)
	sc.PriorExternalRunIDs = map[int]int64{0: 7}

	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
}

func TestTagStep_AlreadyExistsAtExpectedCommit(t *testing.T) {
	prov := &stepProvider{
		createTag: func(_ context.Context, _, _, _, _, _ string) error {
			return errors.New("422 Reference already exists")
		},
		getTagCommitSHA: func(_ context.Context, _, _, tag string) (string, error) {
			if tag != "v1.2.3" {
				t.Errorf("looked up tag %q, want v1.2.3", tag)
			}
			return "abc123", nil
		},
	}

	sc := newSC(&stepStore{}, prov)
	sc.Tag = "v1.2.3"
	sc.Event.CommitSHA = "abc123"

	s := &steps.TagStep{}
	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: expected success when tag already points at our commit, got: %v", err)
	}
}

func TestTagStep_AlreadyExistsAtDifferentCommit(t *testing.T) {
	prov := &stepProvider{
		createTag: func(_ context.Context, _, _, _, _, _ string) error {
			return errors.New("422 Reference already exists")
		},
		getTagCommitSHA: func(_ context.Context, _, _, _ string) (string, error) {
			return "other-sha", nil
		},
	}

	sc := newSC(&stepStore{}, prov)
	sc.Tag = "v1.2.3"
	sc.Event.CommitSHA = "abc123"

	s := &steps.TagStep{}
	err := s.Run(t.Context(), sc)
	if err == nil {
		t.Fatal("expected error for tag collision, got nil")
	}
	if !strings.Contains(err.Error(), "points at other-sha") {
		t.Errorf("error should name the conflicting commit, got: %v", err)
	}
}

func TestTagStep_CreateFailsAndTagAbsent_ReturnsOriginalError(t *testing.T) {
	prov := &stepProvider{
		createTag: func(_ context.Context, _, _, _, _, _ string) error {
			return errors.New("boom")
		},
		// default getTagCommitSHA returns provider.ErrNotFound
	}

	sc := newSC(&stepStore{}, prov)
	sc.Tag = "v1.2.3"
	sc.Event.CommitSHA = "abc123"

	s := &steps.TagStep{}
	err := s.Run(t.Context(), sc)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected original create error, got: %v", err)
	}
}

func TestCreateReleaseStep_AlreadyExists(t *testing.T) {
	prov := &stepProvider{
		getReleaseByTag: func(_ context.Context, _, _, tag string) (string, error) {
			return "https://example.com/releases/" + tag, nil
		},
		createRelease: func(_ context.Context, _, _, _, _, _ string, _, _ bool) (string, error) {
			t.Error("CreateRelease called; existing release must be reused")
			return "", nil
		},
	}

	sc := newSC(&stepStore{}, prov)
	sc.Tag = "v1.2.3"

	s, _ := steps.NewCreateReleaseStep(map[string]any{})
	if err := s.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !strings.Contains(sc.Output, `"existing":true`) {
		t.Errorf("output should mark the release as pre-existing, got: %s", sc.Output)
	}
}
