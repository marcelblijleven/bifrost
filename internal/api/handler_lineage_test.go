package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/sse"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// lineageStore implements only the store methods the lineage flow touches.
// Any other call panics via the embedded nil interface.
type lineageStore struct {
	store.Store

	app              *store.Application
	createdRuns      []*store.PipelineRun
	pendingRunIDs    []uuid.UUID
	forceAdvanceFail bool // simulate a concurrent delivery winning the CAS
}

func (s *lineageStore) AdvanceApplicationHead(_ context.Context, _ uuid.UUID, from, to string) (bool, error) {
	if s.forceAdvanceFail || s.app.LastKnownSHA != from || s.app.HeadState == store.HeadStateBlocked {
		return false, nil
	}
	s.app.LastKnownSHA = to
	return true, nil
}

func (s *lineageStore) BlockApplication(_ context.Context, _ uuid.UUID, reason string) error {
	s.app.HeadState = store.HeadStateBlocked
	s.app.BlockedReason = reason
	return nil
}

func (s *lineageStore) AcceptApplicationHead(_ context.Context, _ uuid.UUID, head string) error {
	s.app.HeadState = store.HeadStateOK
	s.app.LastKnownSHA = head
	s.app.BlockedReason = ""
	return nil
}

func (s *lineageStore) CancelPendingRuns(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	ids := s.pendingRunIDs
	s.pendingRunIDs = nil
	return ids, nil
}

func (s *lineageStore) CreatePipelineRun(_ context.Context, r *store.PipelineRun) error {
	s.createdRuns = append(s.createdRuns, r)
	return nil
}

func (s *lineageStore) GetApplication(_ context.Context, _ uuid.UUID) (*store.Application, error) {
	return s.app, nil
}

// lineageProvider implements only CompareCommits and GetBranchHead.
type lineageProvider struct {
	provider.Provider

	compare    func(base, head string) (provider.CompareStatus, error)
	branchHead string
}

func (p *lineageProvider) CompareCommits(_ context.Context, _, _, base, head string) (provider.CompareStatus, error) {
	if p.compare != nil {
		return p.compare(base, head)
	}
	return "", errors.New("unexpected CompareCommits call")
}

func (p *lineageProvider) GetBranchHead(_ context.Context, _, _, _ string) (string, error) {
	if p.branchHead == "" {
		return "", fmt.Errorf("branch: %w", provider.ErrNotFound)
	}
	return p.branchHead, nil
}

func newLineageHandler(st *lineageStore, prov provider.Provider) *Handler {
	return NewHandler(st, map[string]provider.Provider{"github": prov},
		pipeline.NewRegistry(), "test-secret", "", sse.New())
}

func lineageApp(lastKnown string) *store.Application {
	return &store.Application{
		ID:           uuid.New(),
		Provider:     "github",
		Owner:        "owner",
		Repo:         "repo",
		Branch:       "main",
		LastKnownSHA: lastKnown,
		HeadState:    store.HeadStateOK,
	}
}

func pushEvent(before, after string) provider.PushEvent {
	return provider.PushEvent{
		ProviderID: "github",
		RepoOwner:  "owner",
		RepoName:   "repo",
		Branch:     "main",
		BeforeSHA:  before,
		CommitSHA:  after,
	}
}

func TestLineage_ChainMatch_AdvancesHead(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	h := newLineageHandler(st, &lineageProvider{})

	proceed, _, _ := h.processLineage(t.Context(), st.app, &lineageProvider{}, pushEvent("aaa", "bbb"))
	if !proceed {
		t.Fatal("expected lineage to allow the run")
	}
	if st.app.LastKnownSHA != "bbb" {
		t.Errorf("head = %q, want bbb", st.app.LastKnownSHA)
	}
}

func TestLineage_FirstContact_AdoptsHead(t *testing.T) {
	st := &lineageStore{app: lineageApp("")}
	h := newLineageHandler(st, &lineageProvider{})

	proceed, _, _ := h.processLineage(t.Context(), st.app, &lineageProvider{}, pushEvent("aaa", "bbb"))
	if !proceed {
		t.Fatal("expected lineage to allow the run on first contact")
	}
	if st.app.LastKnownSHA != "bbb" {
		t.Errorf("head = %q, want bbb", st.app.LastKnownSHA)
	}
}

func TestLineage_ForcedPush_Blocks(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa"), pendingRunIDs: []uuid.UUID{uuid.New()}}
	h := newLineageHandler(st, &lineageProvider{})

	ev := pushEvent("aaa", "bbb")
	ev.Forced = true
	proceed, status, _ := h.processLineage(t.Context(), st.app, &lineageProvider{}, ev)
	if proceed {
		t.Fatal("expected lineage to reject a forced push")
	}
	if status != "blocked" {
		t.Errorf("status = %q, want blocked", status)
	}
	if st.app.HeadState != store.HeadStateBlocked {
		t.Errorf("head_state = %q, want blocked", st.app.HeadState)
	}
	if !strings.Contains(st.app.BlockedReason, "Force push detected") {
		t.Errorf("blocked reason should explain the force push, got: %q", st.app.BlockedReason)
	}
	if !strings.Contains(st.app.BlockedReason, "To recover") {
		t.Errorf("blocked reason should include recovery instructions, got: %q", st.app.BlockedReason)
	}
	// A blocked run must be recorded for timeline visibility.
	if len(st.createdRuns) != 1 || st.createdRuns[0].Status != "blocked" {
		t.Errorf("expected one 'blocked' run record, got %+v", st.createdRuns)
	}
}

func TestLineage_MismatchAhead_SyncsOverMissedPushes(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	prov := &lineageProvider{
		compare: func(base, head string) (provider.CompareStatus, error) {
			if base != "aaa" || head != "ccc" {
				t.Errorf("compared %s...%s, want aaa...ccc", base, head)
			}
			return provider.CompareAhead, nil
		},
	}
	h := newLineageHandler(st, prov)

	// The webhook claims 'bbb' was the previous head, but bifrost never saw
	// the aaa→bbb push (missed delivery).
	proceed, _, _ := h.processLineage(t.Context(), st.app, prov, pushEvent("bbb", "ccc"))
	if !proceed {
		t.Fatal("expected lineage to sync over missed pushes")
	}
	if st.app.LastKnownSHA != "ccc" {
		t.Errorf("head = %q, want ccc", st.app.LastKnownSHA)
	}
}

func TestLineage_MismatchStale_Ignored(t *testing.T) {
	for _, status := range []provider.CompareStatus{provider.CompareBehind, provider.CompareIdentical} {
		st := &lineageStore{app: lineageApp("ccc")}
		prov := &lineageProvider{
			compare: func(_, _ string) (provider.CompareStatus, error) { return status, nil },
		}
		h := newLineageHandler(st, prov)

		proceed, result, _ := h.processLineage(t.Context(), st.app, prov, pushEvent("aaa", "bbb"))
		if proceed {
			t.Errorf("%s: expected stale delivery to be ignored", status)
		}
		if result != "stale" {
			t.Errorf("%s: result = %q, want stale", status, result)
		}
		if st.app.HeadState != store.HeadStateOK {
			t.Errorf("%s: stale delivery must not block the application", status)
		}
		if st.app.LastKnownSHA != "ccc" {
			t.Errorf("%s: head = %q, want unchanged ccc", status, st.app.LastKnownSHA)
		}
	}
}

func TestLineage_MismatchDiverged_Blocks(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	prov := &lineageProvider{
		compare: func(_, _ string) (provider.CompareStatus, error) { return provider.CompareDiverged, nil },
	}
	h := newLineageHandler(st, prov)

	proceed, _, _ := h.processLineage(t.Context(), st.app, prov, pushEvent("xxx", "yyy"))
	if proceed {
		t.Fatal("expected diverged history to be rejected")
	}
	if st.app.HeadState != store.HeadStateBlocked {
		t.Errorf("head_state = %q, want blocked", st.app.HeadState)
	}
}

func TestLineage_PreviousHeadGone_Blocks(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	prov := &lineageProvider{
		compare: func(_, _ string) (provider.CompareStatus, error) {
			return "", fmt.Errorf("compare: %w", provider.ErrNotFound)
		},
	}
	h := newLineageHandler(st, prov)

	proceed, _, _ := h.processLineage(t.Context(), st.app, prov, pushEvent("xxx", "yyy"))
	if proceed {
		t.Fatal("expected unknown previous head to be rejected")
	}
	if st.app.HeadState != store.HeadStateBlocked {
		t.Errorf("head_state = %q, want blocked", st.app.HeadState)
	}
	if !strings.Contains(st.app.BlockedReason, "no longer exists") {
		t.Errorf("blocked reason should mention the missing head, got: %q", st.app.BlockedReason)
	}
}

func TestLineage_CompareError_FailsClosedWithoutBlocking(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	prov := &lineageProvider{
		compare: func(_, _ string) (provider.CompareStatus, error) {
			return "", errors.New("api unavailable")
		},
	}
	h := newLineageHandler(st, prov)

	proceed, status, _ := h.processLineage(t.Context(), st.app, prov, pushEvent("xxx", "yyy"))
	if proceed {
		t.Fatal("expected transient compare failure to reject the run")
	}
	if status != "error" {
		t.Errorf("status = %q, want error so the delivery can be retried", status)
	}
	if st.app.HeadState != store.HeadStateOK {
		t.Error("a transient provider error must not block the application")
	}
}

func TestLineage_BlockedApp_RejectsPushes(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	st.app.HeadState = store.HeadStateBlocked
	st.app.BlockedReason = "Force push detected..."
	h := newLineageHandler(st, &lineageProvider{})

	proceed, status, reason := h.processLineage(t.Context(), st.app, &lineageProvider{}, pushEvent("aaa", "bbb"))
	if proceed {
		t.Fatal("expected pushes to a blocked application to be rejected")
	}
	if status != "blocked" || reason == "" {
		t.Errorf("status = %q (reason %q), want blocked with a reason", status, reason)
	}
}

func TestLineage_BranchDeleted_Blocks(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	h := newLineageHandler(st, &lineageProvider{})

	proceed, _, _ := h.processLineage(t.Context(), st.app, &lineageProvider{}, pushEvent("aaa", provider.ZeroSHA))
	if proceed {
		t.Fatal("expected branch deletion to be rejected")
	}
	if st.app.HeadState != store.HeadStateBlocked {
		t.Errorf("head_state = %q, want blocked", st.app.HeadState)
	}
	if !strings.Contains(st.app.BlockedReason, "deleted") {
		t.Errorf("blocked reason should mention deletion, got: %q", st.app.BlockedReason)
	}
}

func TestLineage_ConcurrentDelivery_Superseded(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa"), forceAdvanceFail: true}
	h := newLineageHandler(st, &lineageProvider{})

	proceed, status, _ := h.processLineage(t.Context(), st.app, &lineageProvider{}, pushEvent("aaa", "bbb"))
	if proceed {
		t.Fatal("expected a lost CAS to drop the delivery")
	}
	if status != "superseded" {
		t.Errorf("status = %q, want superseded", status)
	}
	if st.app.HeadState != store.HeadStateOK {
		t.Error("a lost CAS must not block the application")
	}
}

func TestAcceptApplicationHead_UnblocksAndTriggersRun(t *testing.T) {
	st := &lineageStore{app: lineageApp("old-head")}
	st.app.HeadState = store.HeadStateBlocked
	st.app.BlockedReason = "Force push detected..."
	prov := &lineageProvider{branchHead: "live-head"}
	h := newLineageHandler(st, prov)

	req := httptest.NewRequest(http.MethodPost, "/applications/"+st.app.ID.String()+"/head/accept",
		strings.NewReader(`{"trigger_run":true}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", st.app.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.AcceptApplicationHead(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if st.app.HeadState != store.HeadStateOK {
		t.Errorf("head_state = %q, want ok", st.app.HeadState)
	}
	// The baseline must come from the provider's live head, not the stale
	// webhook or the stored value.
	if st.app.LastKnownSHA != "live-head" {
		t.Errorf("head = %q, want live-head", st.app.LastKnownSHA)
	}
	if len(st.createdRuns) != 1 || st.createdRuns[0].CommitSHA != "live-head" || st.createdRuns[0].Status != "pending" {
		t.Errorf("expected a pending run for the accepted head, got %+v", st.createdRuns)
	}
}

func TestAcceptApplicationHead_BranchMissing_Conflict(t *testing.T) {
	st := &lineageStore{app: lineageApp("old-head")}
	st.app.HeadState = store.HeadStateBlocked
	h := newLineageHandler(st, &lineageProvider{ /* branchHead empty → ErrNotFound */ })

	req := httptest.NewRequest(http.MethodPost, "/applications/"+st.app.ID.String()+"/head/accept", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", st.app.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.AcceptApplicationHead(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if st.app.HeadState != store.HeadStateBlocked {
		t.Error("application must stay blocked when the branch cannot be resolved")
	}
}
