package pipeline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// ── mock store ────────────────────────────────────────────────────────────────

type mockStore struct {
	runUpdates   []*store.PipelineRun
	stepCreates  []*store.StepResult
	stepUpdates  []*store.StepResult
	deletedFrom  int
	releasedRuns []uuid.UUID
}

func (m *mockStore) UpdatePipelineRun(_ context.Context, r *store.PipelineRun) error {
	m.runUpdates = append(m.runUpdates, r)
	return nil
}
func (m *mockStore) CreateStepResult(_ context.Context, r *store.StepResult) error {
	m.stepCreates = append(m.stepCreates, r)
	return nil
}
func (m *mockStore) UpdateStepResult(_ context.Context, r *store.StepResult) error {
	m.stepUpdates = append(m.stepUpdates, r)
	return nil
}
func (m *mockStore) DeleteStepResultsFrom(_ context.Context, _ uuid.UUID, fromStepIndex int) error {
	m.deletedFrom = fromStepIndex
	return nil
}

// Remaining Store methods – unused in pipeline tests.
func (m *mockStore) CreateApplication(_ context.Context, _ *store.Application) error { return nil }
func (m *mockStore) GetApplication(_ context.Context, _ uuid.UUID) (*store.Application, error) {
	return nil, nil
}
func (m *mockStore) ListApplicationsByRepo(_ context.Context, _, _, _ string) ([]*store.Application, error) {
	return nil, nil
}
func (m *mockStore) GetRunByTriggerTag(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
	return nil, nil
}
func (m *mockStore) ListApplications(_ context.Context) ([]*store.Application, error) {
	return nil, nil
}
func (m *mockStore) ListApplicationsForUser(_ context.Context, _ uuid.UUID) ([]*store.Application, error) {
	return nil, nil
}
func (m *mockStore) CanUserAccessApplication(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockStore) UpdateApplication(_ context.Context, _ *store.Application) error { return nil }
func (m *mockStore) DeleteApplication(_ context.Context, _ uuid.UUID) error          { return nil }
func (m *mockStore) ListLatestRuns(_ context.Context) (map[uuid.UUID]*store.PipelineRun, error) {
	return nil, nil
}
func (m *mockStore) GrantGroupAccess(_ context.Context, _, _ uuid.UUID) error  { return nil }
func (m *mockStore) RevokeGroupAccess(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockStore) ListApplicationGroups(_ context.Context, _ uuid.UUID) ([]*store.Group, error) {
	return nil, nil
}
func (m *mockStore) CreatePipelineRun(_ context.Context, _ *store.PipelineRun) error { return nil }
func (m *mockStore) GetPipelineRun(_ context.Context, _ uuid.UUID) (*store.PipelineRun, error) {
	return nil, nil
}
func (m *mockStore) ListPipelineRuns(_ context.Context, _ uuid.UUID, _, _ int, _ store.RunFilter) ([]*store.PipelineRun, error) {
	return nil, nil
}
func (m *mockStore) ListStepResults(_ context.Context, _ uuid.UUID) ([]*store.StepResult, error) {
	return nil, nil
}
func (m *mockStore) CreateUser(_ context.Context, _ *store.User) error { return nil }
func (m *mockStore) GetUserByEmail(_ context.Context, _ string) (*store.User, error) {
	return nil, nil
}
func (m *mockStore) GetUserForAuth(_ context.Context, _ string) (*store.User, error) {
	return nil, nil
}
func (m *mockStore) ListUsers(_ context.Context) ([]*store.User, error) { return nil, nil }
func (m *mockStore) CountAdmins(_ context.Context) (int, error)         { return 0, nil }
func (m *mockStore) UpdateUserPassword(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (m *mockStore) SetUserAdmin(_ context.Context, _ uuid.UUID, _ bool) error { return nil }
func (m *mockStore) DeleteUser(_ context.Context, _ uuid.UUID) error           { return nil }
func (m *mockStore) CreateGroup(_ context.Context, _ *store.Group) error       { return nil }
func (m *mockStore) GetGroup(_ context.Context, _ uuid.UUID) (*store.Group, error) {
	return nil, nil
}
func (m *mockStore) ListGroups(_ context.Context) ([]*store.Group, error)        { return nil, nil }
func (m *mockStore) UpdateGroup(_ context.Context, _ *store.Group) error         { return nil }
func (m *mockStore) DeleteGroup(_ context.Context, _ uuid.UUID) error            { return nil }
func (m *mockStore) AddUserToGroup(_ context.Context, _, _ uuid.UUID) error      { return nil }
func (m *mockStore) RemoveUserFromGroup(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockStore) ListGroupMembers(_ context.Context, _ uuid.UUID) ([]*store.User, error) {
	return nil, nil
}
func (m *mockStore) CreateApprovalRequest(_ context.Context, _ *store.ApprovalRequest) error {
	return nil
}
func (m *mockStore) GetApprovalRequest(_ context.Context, _ uuid.UUID) (*store.ApprovalRequest, error) {
	return nil, nil
}
func (m *mockStore) GetPendingApproval(_ context.Context, _ uuid.UUID, _ int) (*store.ApprovalRequest, error) {
	return nil, nil
}
func (m *mockStore) GetApprovalForStep(_ context.Context, _ uuid.UUID, _ int) (*store.ApprovalRequest, error) {
	return nil, nil
}
func (m *mockStore) DeleteApprovalRequestsFrom(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}
func (m *mockStore) ResolveApprovalRequest(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (m *mockStore) SupersedeOlderApprovals(_ context.Context, _ uuid.UUID, _ int, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockStore) ListApprovalRequests(_ context.Context, _ uuid.UUID) ([]*store.ApprovalRequest, error) {
	return nil, nil
}
func (m *mockStore) GetStepResultByExternalRunID(_ context.Context, _ int64) (*store.StepResult, error) {
	return nil, nil
}
func (m *mockStore) ClaimPendingRun(_ context.Context, _ string) (*store.PipelineRun, error) {
	return nil, nil
}
func (m *mockStore) HeartbeatRun(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return true, nil
}
func (m *mockStore) ReapExpiredRuns(_ context.Context) (int, error) { return 0, nil }
func (m *mockStore) AdvanceApplicationHead(_ context.Context, _ uuid.UUID, _, _ string) (bool, error) {
	return true, nil
}
func (m *mockStore) BlockApplication(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (m *mockStore) AcceptApplicationHead(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (m *mockStore) CancelPendingRuns(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockStore) SetStepResultExternalRunID(_ context.Context, _ uuid.UUID, _ int, _ int64) error {
	return nil
}
func (m *mockStore) OverrideStepResult(_ context.Context, _ uuid.UUID, _ int, _, _ string) (bool, error) {
	return true, nil
}
func (m *mockStore) UpdateRunTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (m *mockStore) MarkRunReleased(_ context.Context, id uuid.UUID) error {
	m.releasedRuns = append(m.releasedRuns, id)
	return nil
}
func (m *mockStore) GetLastReleasedRun(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
	return nil, nil
}
func (m *mockStore) ResetStepResultsFrom(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}
func (m *mockStore) CancelRun(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockStore) ResetRunToPending(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *mockStore) GetDashboardStats(_ context.Context) (*store.DashboardStats, error) {
	return &store.DashboardStats{}, nil
}
func (m *mockStore) Close() {}

// ── mock steps ────────────────────────────────────────────────────────────────

type mockStep struct {
	name     string
	runErr   error
	restored bool
}

func (s *mockStep) Name() string { return s.name }
func (s *mockStep) Run(_ context.Context, _ *pipeline.StepContext) error {
	return s.runErr
}

type mockRestorerStep struct {
	mockStep
	restoreErr error
}

func (s *mockRestorerStep) Restore(_ context.Context, _ *pipeline.StepContext) error {
	s.restored = true
	return s.restoreErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newSC() *pipeline.StepContext {
	return &pipeline.StepContext{}
}

func findRunStatus(updates []*store.PipelineRun) string {
	for i := len(updates) - 1; i >= 0; i-- {
		if updates[i].Status != "" {
			return updates[i].Status
		}
	}
	return ""
}

func stepStatuses(updates []*store.StepResult) []string {
	seen := map[uuid.UUID]string{}
	order := []uuid.UUID{}
	for _, u := range updates {
		if _, ok := seen[u.ID]; !ok {
			order = append(order, u.ID)
		}
		seen[u.ID] = u.Status
	}
	out := make([]string, len(order))
	for i, id := range order {
		out[i] = seen[id]
	}
	return out
}

// ── Execute tests ─────────────────────────────────────────────────────────────

func TestExecute_AllSucceed(t *testing.T) {
	st := &mockStore{}
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "a"},
		&mockStep{name: "b"},
		&mockStep{name: "c"},
	})

	err := p.Execute(t.Context(), newSC(), st, uuid.New())
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	status := findRunStatus(st.runUpdates)
	if status != "success" {
		t.Errorf("final run status = %q, want %q", status, "success")
	}
	if len(st.releasedRuns) != 1 {
		t.Errorf("MarkRunReleased called %d times, want 1", len(st.releasedRuns))
	}
}

func TestExecute_StepFails_RunMarkedFailed(t *testing.T) {
	st := &mockStore{}
	boom := errors.New("step b failed")
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "a"},
		&mockStep{name: "b", runErr: boom},
		&mockStep{name: "c"},
	})

	err := p.Execute(t.Context(), newSC(), st, uuid.New())
	if !errors.Is(err, boom) {
		t.Fatalf("Execute: want %v, got %v", boom, err)
	}

	status := findRunStatus(st.runUpdates)
	if status != "failed" {
		t.Errorf("final run status = %q, want %q", status, "failed")
	}
	if len(st.releasedRuns) != 0 {
		t.Errorf("MarkRunReleased called %d times, want 0 on failure", len(st.releasedRuns))
	}
}

func TestExecute_StepFails_SubsequentStepsSkipped(t *testing.T) {
	st := &mockStore{}
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "a"},
		&mockStep{name: "b", runErr: errors.New("boom")},
		&mockStep{name: "c"},
	})

	p.Execute(t.Context(), newSC(), st, uuid.New()) //nolint:errcheck

	// c should have status "skipped" in final updates
	finalStatuses := map[string]string{}
	for _, u := range st.stepUpdates {
		finalStatuses[u.StepName] = u.Status
	}
	if finalStatuses["c"] != "skipped" {
		t.Errorf("step c status = %q, want %q", finalStatuses["c"], "skipped")
	}
}

func TestExecute_Superseded_RunMarkedSuperseded(t *testing.T) {
	st := &mockStore{}
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "approval", runErr: pipeline.ErrSuperseded},
	})

	err := p.Execute(t.Context(), newSC(), st, uuid.New())
	if !errors.Is(err, pipeline.ErrSuperseded) {
		t.Fatalf("Execute: want ErrSuperseded, got %v", err)
	}

	status := findRunStatus(st.runUpdates)
	if status != "superseded" {
		t.Errorf("final run status = %q, want %q", status, "superseded")
	}

	// The approval step itself must be skipped (not failed) so the frontend
	// shows no error — the approval block already shows the superseded state.
	finalStatuses := make(map[string]string)
	for _, r := range st.stepUpdates {
		finalStatuses[r.StepName] = r.Status
	}
	if finalStatuses["approval"] != "skipped" {
		t.Errorf("superseded step status = %q, want %q", finalStatuses["approval"], "skipped")
	}
	for _, r := range st.stepUpdates {
		if r.StepName == "approval" && r.ErrorMessage != "" {
			t.Errorf("superseded step ErrorMessage = %q, want empty", r.ErrorMessage)
		}
	}
}

func TestExecute_StepTimings(t *testing.T) {
	st := &mockStore{}
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "a"},
	})

	before := time.Now()
	p.Execute(t.Context(), newSC(), st, uuid.New()) //nolint:errcheck
	after := time.Now()

	// run should have been marked "running" with a StartedAt timestamp
	var startedAt *time.Time
	for _, u := range st.runUpdates {
		if u.Status == "running" {
			startedAt = u.StartedAt
		}
	}
	if startedAt == nil {
		t.Fatal("run never marked running with a StartedAt time")
	}
	if startedAt.Before(before) || startedAt.After(after) {
		t.Errorf("StartedAt %v not in expected window [%v, %v]", *startedAt, before, after)
	}
}

// ── ExecuteFrom tests ─────────────────────────────────────────────────────────

func TestExecuteFrom_ResumesFromStep(t *testing.T) {
	st := &mockStore{}
	restorer := &mockRestorerStep{mockStep: mockStep{name: "a"}}
	p := pipeline.New([]pipeline.Step{
		restorer,
		&mockStep{name: "b"},
		&mockStep{name: "c"},
	})

	err := p.ExecuteFrom(t.Context(), newSC(), st, uuid.New(), 1)
	if err != nil {
		t.Fatalf("ExecuteFrom: unexpected error: %v", err)
	}

	if !restorer.restored {
		t.Error("expected step a (Restorer) to be restored before fromStep")
	}

	status := findRunStatus(st.runUpdates)
	if status != "success" {
		t.Errorf("final run status = %q, want %q", status, "success")
	}
	if len(st.releasedRuns) != 1 {
		t.Errorf("MarkRunReleased called %d times, want 1", len(st.releasedRuns))
	}
}

func TestExecuteFrom_DeletesStepResultsFromIndex(t *testing.T) {
	st := &mockStore{}
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "a"},
		&mockStep{name: "b"},
	})

	p.ExecuteFrom(t.Context(), newSC(), st, uuid.New(), 1) //nolint:errcheck

	if st.deletedFrom != 1 {
		t.Errorf("DeleteStepResultsFrom called with %d, want 1", st.deletedFrom)
	}
}

func TestExecuteFrom_FailureAfterResume(t *testing.T) {
	st := &mockStore{}
	p := pipeline.New([]pipeline.Step{
		&mockStep{name: "a"},
		&mockStep{name: "b", runErr: errors.New("b failed")},
		&mockStep{name: "c"},
	})

	err := p.ExecuteFrom(t.Context(), newSC(), st, uuid.New(), 1)
	if err == nil {
		t.Fatal("expected error from step b, got nil")
	}

	status := findRunStatus(st.runUpdates)
	if status != "failed" {
		t.Errorf("final run status = %q, want %q", status, "failed")
	}

	finalStatuses := map[string]string{}
	for _, u := range st.stepUpdates {
		finalStatuses[u.StepName] = u.Status
	}
	if finalStatuses["c"] != "skipped" {
		t.Errorf("step c status = %q, want %q", finalStatuses["c"], "skipped")
	}
}

func TestExecuteFrom_NonRestorerStepsSkippedDuringRestore(t *testing.T) {
	st := &mockStore{}
	sideEffect := &mockStep{name: "tag"} // not a Restorer
	restorer := &mockRestorerStep{mockStep: mockStep{name: "semver"}}
	p := pipeline.New([]pipeline.Step{
		restorer,
		sideEffect,
		&mockStep{name: "target"},
	})

	p.ExecuteFrom(t.Context(), newSC(), st, uuid.New(), 2) //nolint:errcheck

	// Only the restorer should be restored, not the non-restorer
	if !restorer.restored {
		t.Error("expected semver step to be restored")
	}
	// sideEffect (tag) has no Restore; its Run should not be called in the restore phase
	// The only way to verify is indirectly: no step updates for "tag" from before fromStep
	for _, u := range st.stepUpdates {
		if u.StepName == "tag" {
			// tag step was only created and run as a new step if it were in range [fromStep:]
			// Since fromStep=2 and tag is at index 1, it should not appear in creates
			for _, c := range st.stepCreates {
				if c.StepName == "tag" {
					t.Error("tag step should not be re-created during ExecuteFrom (it's before fromStep)")
				}
			}
		}
	}
}

// leaseLosingStep simulates the heartbeat cancelling the run's context with
// ErrLeaseLost while the step executes.
type leaseLosingStep struct {
	cancel context.CancelCauseFunc
}

func (s *leaseLosingStep) Name() string { return "lease-loser" }
func (s *leaseLosingStep) Run(ctx context.Context, _ *pipeline.StepContext) error {
	s.cancel(pipeline.ErrLeaseLost)
	return ctx.Err()
}

func TestExecute_LeaseLost_AbandonsWithoutPersisting(t *testing.T) {
	st := &mockStore{}
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)

	p := pipeline.New([]pipeline.Step{
		&leaseLosingStep{cancel: cancel},
		&mockStep{name: "after"},
	})

	err := p.Execute(ctx, newSC(), st, uuid.New())
	if !errors.Is(err, pipeline.ErrLeaseLost) {
		t.Fatalf("Execute: want ErrLeaseLost, got %v", err)
	}

	// The run's final status must not be written: another instance owns the
	// run now. The only run update is the initial "running" mark.
	for _, u := range st.runUpdates {
		if u.CompletedAt != nil {
			t.Errorf("run completion persisted after lease loss: status %q", u.Status)
		}
	}
	// No step result may be finalised or skipped after the lease was lost;
	// only the first step's "running" transition is allowed.
	for _, u := range st.stepUpdates {
		if u.Status != "running" {
			t.Errorf("step %q persisted status %q after lease loss", u.StepName, u.Status)
		}
	}
	if len(st.releasedRuns) != 0 {
		t.Errorf("MarkRunReleased called %d times after lease loss, want 0", len(st.releasedRuns))
	}
}

func TestExecute_UserCancelled_PersistsCancelledState(t *testing.T) {
	st := &mockStore{}
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)

	p := pipeline.New([]pipeline.Step{
		&leaseLosingStep{cancel: func(_ error) { cancel(nil) }}, // plain cancellation
		&mockStep{name: "after"},
	})

	err := p.Execute(ctx, newSC(), st, uuid.New())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute: want context.Canceled, got %v", err)
	}

	// Unlike a lost lease, a user cancellation must persist the final state
	// even though the run's context is already cancelled.
	if status := findRunStatus(st.runUpdates); status != "cancelled" {
		t.Errorf("final run status = %q, want %q", status, "cancelled")
	}
}
