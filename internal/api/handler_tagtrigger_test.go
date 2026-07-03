package api

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/store"
)

func (s *lineageStore) GetRunByTriggerTag(_ context.Context, applicationID uuid.UUID, tag string) (*store.PipelineRun, error) {
	for _, r := range s.createdRuns {
		if r.ApplicationID == applicationID && r.TriggerTag == tag {
			return r, nil
		}
	}
	return nil, nil
}

// tagTriggerProvider extends the lineage fakes with tag resolution.
type tagTriggerProvider struct {
	lineageProvider
	tagCommit string // resolved commit for any tag; empty → ErrNotFound
}

func (p *tagTriggerProvider) GetTagCommitSHA(_ context.Context, _, _, _ string) (string, error) {
	if p.tagCommit == "" {
		return "", provider.ErrNotFound
	}
	return p.tagCommit, nil
}

func tagApp() *store.Application {
	app := lineageApp("")
	app.Name = "svc"
	app.TriggerType = store.TriggerTag
	app.TagPattern = "v*"
	return app
}

func tagEvent(tag, sha string) provider.PushEvent {
	return provider.PushEvent{
		ProviderID: "github",
		RepoOwner:  "owner",
		RepoName:   "repo",
		TagName:    tag,
		CommitSHA:  sha,
		AuthorName: "alice",
	}
}

// reachableProvider resolves every tag to commit and reports it reachable.
func reachableProvider(commit string) *tagTriggerProvider {
	return &tagTriggerProvider{
		tagCommit: commit,
		lineageProvider: lineageProvider{
			branchHead: "head",
			compare: func(base, head string) (provider.CompareStatus, error) {
				return provider.CompareAhead, nil
			},
		},
	}
}

func TestTagTrigger_MatchingTag_QueuesRun(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})

	res := h.processEventForApp(t.Context(), st.app, reachableProvider("abc"), tagEvent("v1.2.3", "tagobj"))
	if res.Status != "queued" {
		t.Fatalf("status = %q (reason %q), want queued", res.Status, res.Reason)
	}
	if len(st.createdRuns) != 1 {
		t.Fatalf("expected one run, got %d", len(st.createdRuns))
	}
	run := st.createdRuns[0]
	if run.CommitSHA != "abc" {
		t.Errorf("run.CommitSHA = %q, want resolved commit abc", run.CommitSHA)
	}
	if run.TriggerTag != "v1.2.3" || run.Tag != "v1.2.3" {
		t.Errorf("run tag fields = (%q, %q), want v1.2.3", run.Tag, run.TriggerTag)
	}
	if run.Branch != "main" {
		t.Errorf("run.Branch = %q, want the application branch", run.Branch)
	}
}

func TestTagTrigger_NonMatchingTag_Ignored(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})

	res := h.processEventForApp(t.Context(), st.app, reachableProvider("abc"), tagEvent("frontend-v1.0.0", "x"))
	if res.Status != "ignored" {
		t.Fatalf("status = %q, want ignored", res.Status)
	}
	if len(st.createdRuns) != 0 {
		t.Errorf("expected no runs, got %d", len(st.createdRuns))
	}
}

func TestTagTrigger_TagDeletion_Ignored(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})

	res := h.processEventForApp(t.Context(), st.app, reachableProvider("abc"), tagEvent("v1.2.3", provider.ZeroSHA))
	if res.Status != "ignored" {
		t.Fatalf("status = %q, want ignored", res.Status)
	}
}

func TestTagTrigger_DuplicateDelivery_SameCommit(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})
	prov := reachableProvider("abc")

	first := h.processEventForApp(t.Context(), st.app, prov, tagEvent("v1.2.3", "tagobj"))
	if first.Status != "queued" {
		t.Fatalf("first delivery: status = %q, want queued", first.Status)
	}
	second := h.processEventForApp(t.Context(), st.app, prov, tagEvent("v1.2.3", "tagobj"))
	if second.Status != "duplicate" {
		t.Fatalf("second delivery: status = %q, want duplicate", second.Status)
	}
	if len(st.createdRuns) != 1 {
		t.Errorf("expected one run, got %d", len(st.createdRuns))
	}
}

func TestTagTrigger_RecreatedTag_DifferentCommit_Blocks(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})

	if res := h.processEventForApp(t.Context(), st.app, reachableProvider("abc"), tagEvent("v1.2.3", "tagobj")); res.Status != "queued" {
		t.Fatalf("first delivery: status = %q, want queued", res.Status)
	}

	// The tag was deleted and recreated pointing at another commit.
	res := h.processEventForApp(t.Context(), st.app, reachableProvider("evil"), tagEvent("v1.2.3", "tagobj2"))
	if res.Status != "blocked" {
		t.Fatalf("status = %q (reason %q), want blocked", res.Status, res.Reason)
	}
	if st.app.HeadState != store.HeadStateBlocked {
		t.Error("application must be blocked after a tag recreation")
	}
	if !strings.Contains(st.app.BlockedReason, "recreated") {
		t.Errorf("blocked reason should mention the recreation, got: %q", st.app.BlockedReason)
	}
}

func TestTagTrigger_UnreachableCommit_SkipsWithoutClaimingTag(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})
	unreachable := &tagTriggerProvider{
		tagCommit: "feature-commit",
		lineageProvider: lineageProvider{
			branchHead: "head",
			compare: func(_, _ string) (provider.CompareStatus, error) {
				return provider.CompareDiverged, nil
			},
		},
	}

	res := h.processEventForApp(t.Context(), st.app, unreachable, tagEvent("v1.2.3", "tagobj"))
	if res.Status != "skipped" {
		t.Fatalf("status = %q (reason %q), want skipped", res.Status, res.Reason)
	}
	if st.app.HeadState != store.HeadStateOK {
		t.Error("an unreachable tag must not block the application")
	}
	if len(st.createdRuns) != 1 || st.createdRuns[0].Status != "skipped" {
		t.Fatalf("expected one skipped run record, got %+v", st.createdRuns)
	}
	// The skipped record must not claim the trigger tag: once the commit is
	// merged, delivering the same tag again should start a run.
	if st.createdRuns[0].TriggerTag != "" {
		t.Error("skipped run must not record the trigger tag")
	}
	if again := h.processEventForApp(t.Context(), st.app, reachableProvider("feature-commit"), tagEvent("v1.2.3", "tagobj")); again.Status != "queued" {
		t.Errorf("after merge: status = %q, want queued", again.Status)
	}
}

func TestTagTrigger_BranchPush_Ignored(t *testing.T) {
	st := &lineageStore{app: tagApp()}
	h := newLineageHandler(st, &lineageProvider{})

	res := h.processEventForApp(t.Context(), st.app, &lineageProvider{}, pushEvent("aaa", "bbb"))
	if res.Status != "ignored" {
		t.Fatalf("status = %q, want ignored for branch pushes to tag-triggered apps", res.Status)
	}
}

func TestPushApp_TagPush_Ignored(t *testing.T) {
	st := &lineageStore{app: lineageApp("aaa")}
	h := newLineageHandler(st, &lineageProvider{})

	res := h.processEventForApp(t.Context(), st.app, &lineageProvider{}, tagEvent("v1.2.3", "abc"))
	if res.Status != "ignored" {
		t.Fatalf("status = %q, want ignored for tag pushes to push-triggered apps", res.Status)
	}
}

func TestValidateApplication_TriggerRules(t *testing.T) {
	cases := []struct {
		name    string
		app     store.Application
		wantErr string
	}{
		{
			name: "push with tag pattern",
			app:  store.Application{TriggerType: store.TriggerPush, TagPattern: "v*"},

			wantErr: "tag_pattern",
		},
		{
			name:    "tag without pattern",
			app:     store.Application{TriggerType: store.TriggerTag, Branch: "main"},
			wantErr: "tag_pattern",
		},
		{
			name:    "tag without branch",
			app:     store.Application{TriggerType: store.TriggerTag, TagPattern: "v*"},
			wantErr: "branch",
		},
		{
			name: "tag with semver step",
			app: store.Application{
				TriggerType: store.TriggerTag, TagPattern: "v*", Branch: "main",
				PipelineSteps: []store.StepConfig{{Type: "semver"}},
			},
			wantErr: "semver",
		},
		{
			name:    "unknown trigger",
			app:     store.Application{TriggerType: "cron"},
			wantErr: "unknown trigger type",
		},
		{
			name: "valid tag app",
			app: store.Application{
				TriggerType: store.TriggerTag, TagPattern: "v*", Branch: "main",
				PipelineSteps: []store.StepConfig{{Type: "changelog"}, {Type: "create_release"}},
			},
		},
		{
			name: "empty trigger defaults to push",
			app:  store.Application{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateApplication(&c.app)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if c.app.TriggerType == "" {
					t.Error("trigger type must be normalised, got empty")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want mention of %q", err, c.wantErr)
			}
		})
	}
}
