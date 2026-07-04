package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/marcelblijleven/bifrost/internal/auth"
	"github.com/marcelblijleven/bifrost/internal/crypto"
	"github.com/marcelblijleven/bifrost/internal/pathmatch"
	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/sse"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// heartbeatInterval is how often a running run's lease is extended. Must be
// comfortably below the store's lease duration so a couple of missed beats
// don't lose the run to the reaper.
const heartbeatInterval = 20 * time.Second

// Handler holds the dependencies for all HTTP handlers.
type Handler struct {
	store      store.Store
	providers  map[string]provider.Provider
	registry   *pipeline.Registry
	jwtSecret  string
	publicURL  string        // externally reachable URL of this Bifrost instance (no trailing slash)
	instanceID string        // identifies this process as the owner of claimed runs
	pollNow    chan struct{} // capacity-1 wake signal for the run poller
	broker     *sse.Broker

	cancelMu       sync.Mutex
	cancels        map[uuid.UUID]context.CancelCauseFunc // per-run cancel funcs for running goroutines
	approvalWakers sync.Map                              // uuid.UUID → chan struct{}

	loginLimiter *loginLimiter
}

// NewHandler creates a Handler. Call Start before serving requests.
func NewHandler(st store.Store, providers map[string]provider.Provider, reg *pipeline.Registry, jwtSecret, publicURL string, broker *sse.Broker) *Handler {
	return &Handler{
		store:        st,
		providers:    providers,
		registry:     reg,
		jwtSecret:    jwtSecret,
		publicURL:    strings.TrimRight(publicURL, "/"),
		instanceID:   newInstanceID(),
		pollNow:      make(chan struct{}, 1),
		broker:       broker,
		cancels:      make(map[uuid.UUID]context.CancelCauseFunc),
		loginLimiter: newLoginLimiter(),
	}
}

// newInstanceID returns an identifier unique to this process, used as the
// owner of run leases. Hostname keeps it recognisable (pod name on k8s); the
// random suffix disambiguates restarts and same-host processes.
func newInstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "bifrost"
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", host, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", host, hex.EncodeToString(buf))
}

func (h *Handler) encryptSecret(s string) (string, error) {
	return crypto.Encrypt(h.jwtSecret, s)
}

func (h *Handler) decryptSecret(s string) (string, error) {
	return crypto.Decrypt(h.jwtSecret, s)
}

func (h *Handler) registerCancel(id uuid.UUID, fn context.CancelCauseFunc) {
	h.cancelMu.Lock()
	h.cancels[id] = fn
	h.cancelMu.Unlock()
}

func (h *Handler) deregisterCancel(id uuid.UUID) {
	h.cancelMu.Lock()
	delete(h.cancels, id)
	h.cancelMu.Unlock()
}

func (h *Handler) cancelRun(id uuid.UUID) bool {
	h.cancelMu.Lock()
	defer h.cancelMu.Unlock()
	fn, ok := h.cancels[id]
	if ok {
		fn(nil) // nil cause → context.Canceled, treated as user cancellation
		delete(h.cancels, id)
	}
	return ok
}

// Start begins polling for pending runs. Exits when ctx is cancelled.
//
// Each poll first reaps runs whose ownership lease expired (their instance
// crashed or lost its database connection), resetting them to pending, then
// claims and executes claimable pending runs. Every instance does both, so
// any live instance recovers a dead one's runs within a lease duration.
func (h *Handler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			reaped, err := h.store.ReapExpiredRuns(ctx)
			if err != nil {
				slog.Error("poller: reap expired runs", "err", err)
			} else if reaped > 0 {
				slog.Info("poller: reset expired runs to pending", "count", reaped)
			}
			// Drain all claimable runs so a burst of webhooks starts immediately.
			for {
				run, err := h.store.ClaimPendingRun(ctx, h.instanceID)
				if err != nil {
					slog.Error("poller: claim error", "err", err)
					break
				}
				if run == nil {
					break
				}
				go h.executeRun(context.Background(), run)
			}
			select {
			case <-ctx.Done():
				return
			case <-h.pollNow:
			case <-ticker.C:
			}
		}
	}()
}

// heartbeatRun periodically extends the lease on a claimed run. When the lease
// is lost — the run was cancelled via the API (possibly from another instance)
// or reaped after this instance stalled — it cancels the run's context with
// ErrLeaseLost or a plain cancellation respectively.
func (h *Handler) heartbeatRun(ctx context.Context, runID uuid.UUID, cancel context.CancelCauseFunc) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := h.store.HeartbeatRun(context.Background(), runID, h.instanceID)
			if err != nil {
				// Transient store error: keep going; the lease tolerates
				// a couple of missed beats before the reaper takes over.
				slog.Warn("heartbeat failed", "run_id", runID, "err", err)
				continue
			}
			if ok {
				continue
			}
			// Lease lost. Distinguish an external cancellation (persist the
			// cancelled state) from losing ownership (touch nothing: another
			// instance owns the run now).
			run, err := h.store.GetPipelineRun(context.Background(), runID)
			if err == nil && run.Status == "cancelled" {
				slog.Info("run cancelled externally; stopping", "run_id", runID)
				cancel(nil)
				return
			}
			slog.Warn("run lease lost; stopping execution", "run_id", runID)
			cancel(pipeline.ErrLeaseLost)
			return
		}
	}
}

func (h *Handler) executeRun(ctx context.Context, run *store.PipelineRun) {
	ctx, cancel := context.WithCancelCause(ctx)
	h.registerCancel(run.ID, cancel)
	waker := make(chan struct{}, 1)
	h.approvalWakers.Store(run.ID, waker)
	defer func() {
		cancel(nil)
		h.deregisterCancel(run.ID)
		h.approvalWakers.Delete(run.ID)
	}()

	// Keep the ownership lease alive while the run executes; stops the run
	// when the lease is lost (external cancel, or reaped after a stall).
	go h.heartbeatRun(ctx, run.ID, cancel)

	app, err := h.store.GetApplication(ctx, run.ApplicationID)
	if err != nil {
		slog.Error("execute: application not found", "run_id", run.ID, "err", err)
		h.store.UpdatePipelineRun(ctx, &store.PipelineRun{ID: run.ID, Status: "failed"}) //nolint:errcheck
		return
	}
	prov, ok := h.providers[app.Provider]
	if !ok {
		slog.Error("execute: provider not configured", "run_id", run.ID, "provider", app.Provider)
		h.store.UpdatePipelineRun(ctx, &store.PipelineRun{ID: run.ID, Status: "failed"}) //nolint:errcheck
		return
	}
	steps, err := h.registry.Build(app.PipelineSteps, presatisfiedSteps(app)...)
	if err != nil {
		slog.Error("execute: build pipeline steps", "run_id", run.ID, "err", err)
		h.store.UpdatePipelineRun(ctx, &store.PipelineRun{ID: run.ID, Status: "failed"}) //nolint:errcheck
		return
	}

	// Determine where to resume. For a fresh run there are no step results and
	// fromStep stays 0. For a recovered run the step results tell us which step
	// was interrupted. A step left in 'running' state was interrupted mid-flight;
	// if it had already dispatched an external workflow, remember the run ID so
	// the step re-attaches instead of dispatching a second time. (User-initiated
	// retries reset steps to 'pending' first, so they dispatch fresh.)
	fromStep := 0
	priorExternalRunIDs := make(map[int]int64)
	if stepResults, err := h.store.ListStepResults(ctx, run.ID); err == nil {
		for _, s := range stepResults {
			// An overridden step is a failed step a human decided to accept;
			// it counts as satisfied so the run resumes after it.
			if s.Status != "success" && s.Status != "overridden" {
				fromStep = s.StepIndex
				if s.Status == "running" && s.ExternalRunID != nil {
					priorExternalRunIDs[s.StepIndex] = *s.ExternalRunID
				}
				break
			}
			fromStep = s.StepIndex + 1
		}
	}

	sc := &pipeline.StepContext{
		Event: provider.PushEvent{
			ProviderID: app.Provider,
			RepoOwner:  app.Owner,
			RepoName:   app.Repo,
			CommitSHA:  run.CommitSHA,
			CommitMsg:  run.CommitMessage,
			Branch:     run.Branch,
			TagName:    run.TriggerTag,
			AuthorName: run.TriggeredBy,
		},
		Provider:            prov,
		Application:         *app,
		Store:               h.store,
		RunID:               run.ID,
		ApplicationID:       run.ApplicationID,
		WakeApproval:        waker,
		PriorExternalRunIDs: priorExternalRunIDs,
	}
	// Tag-triggered runs have no semver step; seed the tag from the trigger.
	if run.TriggerTag != "" {
		sc.Tag = run.Tag
	}

	p := pipeline.New(steps)
	p.Notify = func() { h.broker.Notify(run.ID) }

	runningRuns.Inc()
	defer runningRuns.Dec()

	var runErr error
	if fromStep > 0 {
		runErr = p.ExecuteFrom(ctx, sc, h.store, run.ID, fromStep)
	} else {
		runErr = p.Execute(ctx, sc, h.store, run.ID)
	}
	if runErr != nil {
		slog.Error("pipeline failed", "run_id", run.ID, "err", runErr)
	}

	// Reload the run to get the final status and timing set by the pipeline.
	if finalRun, err := h.store.GetPipelineRun(context.Background(), run.ID); err == nil {
		runsTotal.WithLabelValues(finalRun.Status).Inc()
		if finalRun.StartedAt != nil && finalRun.CompletedAt != nil {
			runDuration.WithLabelValues(finalRun.Status).Observe(
				finalRun.CompletedAt.Sub(*finalRun.StartedAt).Seconds())
		}
		if (finalRun.Status == "failed" || finalRun.Status == "cancelled") &&
			app.Notifications.OnFailureURL != "" {
			go h.fireFailureNotification(app.Notifications, finalRun)
		}
	}
}

func (h *Handler) fireFailureNotification(n store.NotificationConfig, run *store.PipelineRun) {
	payload := map[string]any{
		"event":          "pipeline.failure",
		"run_id":         run.ID.String(),
		"application_id": run.ApplicationID.String(),
		"status":         run.Status,
		"tag":            run.Tag,
		"commit_sha":     run.CommitSHA,
		"branch":         run.Branch,
		"triggered_by":   run.TriggeredBy,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		n.OnFailureURL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("failure notification: build request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("failure notification: request failed", "url", n.OnFailureURL, "err", err)
		return
	}
	defer resp.Body.Close()
	slog.Info("failure notification: sent", "url", n.OnFailureURL, "status", resp.StatusCode)
}

// ── Webhook ───────────────────────────────────────────────────────────────────

// webhookResult is the per-application outcome of one webhook delivery.
type webhookResult struct {
	ApplicationID uuid.UUID `json:"application_id"`
	Application   string    `json:"application"`
	// Status: queued, skipped, ignored, blocked, stale, superseded, duplicate, error.
	Status string `json:"status"`
	RunID  string `json:"run_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// HandleWebhook receives a push event, validates it against every application
// registered for the repository, and fans it out to each of them.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	prov, ok := h.providers[providerID]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown provider: %s", providerID))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Peek at owner/repo to look up the applications' secrets.
	var peek struct {
		Repository struct {
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &peek); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	owner := peek.Repository.Owner.Login
	repo := peek.Repository.Name
	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "missing repository owner or name in payload")
		return
	}

	applications, err := h.store.ListApplicationsByRepo(r.Context(), providerID, owner, repo)
	if err != nil {
		slog.Error("list applications for webhook",
			"provider", providerID, "owner", owner, "repo", repo, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to look up applications")
		return
	}
	if len(applications) == 0 {
		slog.Warn("no application registered for webhook",
			"provider", providerID, "owner", owner, "repo", repo)
		writeError(w, http.StatusNotFound, "application not registered")
		return
	}

	// The event fans out to every application whose secret validates the
	// signature; apps of one repository normally share a secret, but drift is
	// tolerated.
	var event provider.PushEvent
	var matched []*store.Application
	sawValidNonPush := false
	for _, app := range applications {
		plainSecret, err := h.decryptSecret(app.WebhookSecret)
		if err != nil {
			slog.Warn("failed to decrypt webhook secret", "provider", providerID, "app", app.ID, "err", err)
			continue
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		ev, err := prov.ParseWebhook(r, plainSecret)
		if errors.Is(err, provider.ErrNotPushEvent) {
			sawValidNonPush = true
			continue
		}
		if err != nil {
			slog.Debug("webhook signature did not validate for application",
				"provider", providerID, "app", app.ID, "err", err)
			continue
		}
		event = ev
		matched = append(matched, app)
	}

	if len(matched) == 0 {
		if sawValidNonPush {
			// The signature was valid; check for other handled event types.
			eventType := firstNonEmpty(
				r.Header.Get("X-GitHub-Event"),
				r.Header.Get("X-Gitea-Event"),
				r.Header.Get("X-Forgejo-Event"),
			)
			if wre, werr := prov.ParseWorkflowRun(eventType, body); werr == nil && wre.Action == "completed" {
				h.handleWorkflowRunCompleted(r.Context(), wre)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		slog.Warn("webhook validation failed", "provider", providerID, "owner", owner, "repo", repo)
		writeError(w, http.StatusUnauthorized, "webhook validation failed")
		return
	}

	results := make([]webhookResult, 0, len(matched))
	queued := false
	failed := false
	for _, app := range matched {
		res := h.processEventForApp(r.Context(), app, prov, event)
		queued = queued || res.Status == "queued"
		failed = failed || res.Status == "error"
		results = append(results, res)
	}

	if queued {
		select {
		case h.pollNow <- struct{}{}:
		default:
		}
	}

	// 502 makes the provider redeliver; apps that already handled the event
	// treat the redelivery as stale or duplicate.
	status := http.StatusOK
	switch {
	case failed:
		status = http.StatusBadGateway
	case queued:
		status = http.StatusAccepted
	}
	writeJSON(w, status, map[string]any{"results": results})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// processEventForApp routes one validated push event to one application.
func (h *Handler) processEventForApp(ctx context.Context, app *store.Application, prov provider.Provider, event provider.PushEvent) webhookResult {
	res := webhookResult{ApplicationID: app.ID, Application: app.Name}

	if event.TagName != "" {
		if app.TriggerType != store.TriggerTag {
			res.Status, res.Reason = "ignored", "application does not trigger on tags"
			return res
		}
		return h.processTagTrigger(ctx, res, app, prov, event)
	}

	if app.TriggerType == store.TriggerTag {
		res.Status, res.Reason = "ignored", "application triggers on tags, not branch pushes"
		return res
	}
	if event.Branch != app.Branch {
		res.Status, res.Reason = "ignored", "push to non-target branch"
		return res
	}

	if proceed, status, reason := h.processLineage(ctx, app, prov, event); !proceed {
		res.Status, res.Reason = status, reason
		return res
	}

	if kind, reason := shouldSkip(app.SkipConditions, event); kind != skipNone {
		slog.Info("skipping pipeline run", "app", app.ID, "reason", reason, "sha", event.CommitSHA)
		// Path-based skips are routing and would drown the run list; only
		// commit-message opt-outs are recorded.
		if kind == skipCommitPattern {
			now := time.Now()
			skipped := &store.PipelineRun{
				ID:            uuid.New(),
				ApplicationID: app.ID,
				CommitSHA:     event.CommitSHA,
				ParentSHA:     event.BeforeSHA,
				CommitMessage: event.CommitMsg,
				Branch:        event.Branch,
				TriggeredBy:   event.AuthorName,
				Status:        "skipped",
				StartedAt:     &now,
				CompletedAt:   &now,
			}
			if err := h.store.CreatePipelineRun(ctx, skipped); err != nil {
				slog.Error("create skipped run record", "err", err)
			}
		}
		res.Status, res.Reason = "skipped", reason
		return res
	}

	run := &store.PipelineRun{
		ID:            uuid.New(),
		ApplicationID: app.ID,
		CommitSHA:     event.CommitSHA,
		ParentSHA:     event.BeforeSHA,
		CommitMessage: event.CommitMsg,
		Branch:        event.Branch,
		TriggeredBy:   event.AuthorName,
		Status:        "pending",
	}
	if err := h.store.CreatePipelineRun(ctx, run); err != nil {
		slog.Error("create pipeline run", "app", app.ID, "err", err)
		res.Status, res.Reason = "error", "failed to create pipeline run"
		return res
	}
	res.Status, res.RunID = "queued", run.ID.String()
	return res
}

// processTagTrigger handles a tag push for a tag-triggered application: the
// pushed tag is the release. The tag's commit must be reachable from the
// application's branch, and a tag can trigger at most one run.
func (h *Handler) processTagTrigger(ctx context.Context, res webhookResult, app *store.Application, prov provider.Provider, event provider.PushEvent) webhookResult {
	tag := event.TagName

	if event.CommitSHA == provider.ZeroSHA {
		res.Status, res.Reason = "ignored", "tag deleted"
		return res
	}
	if !pathmatch.Match(app.TagPattern, tag) {
		res.Status, res.Reason = "ignored", "tag does not match the application's tag pattern"
		return res
	}
	if app.HeadState == store.HeadStateBlocked {
		slog.Warn("webhook ignored: application blocked", "app", app.ID, "tag", tag)
		res.Status, res.Reason = "blocked", app.BlockedReason
		return res
	}

	// For annotated tags the payload carries the tag object SHA, not the commit.
	commitSHA, err := prov.GetTagCommitSHA(ctx, app.Owner, app.Repo, tag)
	if err != nil {
		slog.Error("tag trigger: resolve tag commit", "app", app.ID, "tag", tag, "err", err)
		res.Status, res.Reason = "error", "could not resolve the tag's commit; redeliver the webhook"
		return res
	}

	if existing, err := h.store.GetRunByTriggerTag(ctx, app.ID, tag); err != nil {
		slog.Error("tag trigger: look up existing run", "app", app.ID, "tag", tag, "err", err)
		res.Status, res.Reason = "error", "could not check for an existing run; redeliver the webhook"
		return res
	} else if existing != nil {
		if existing.CommitSHA == commitSHA {
			res.Status, res.Reason = "duplicate", "a run for this tag already exists"
			return res
		}
		// Recreated tag at another commit: the tag equivalent of a force push.
		blockEvent := event
		blockEvent.CommitSHA = commitSHA
		blockEvent.Branch = app.Branch
		h.blockApp(ctx, app, blockEvent, fmt.Sprintf(
			"Tag %q was recreated: it previously triggered a run at %s but now points at %s.",
			tag, shortSHA(existing.CommitSHA), shortSHA(commitSHA)))
		res.Status, res.Reason = "blocked", "tag recreated at a different commit"
		return res
	}

	branchHead, err := prov.GetBranchHead(ctx, app.Owner, app.Repo, app.Branch)
	if err != nil {
		slog.Error("tag trigger: get branch head", "app", app.ID, "branch", app.Branch, "err", err)
		res.Status, res.Reason = "error", "could not resolve the application branch head; redeliver the webhook"
		return res
	}
	reach, err := prov.CompareCommits(ctx, app.Owner, app.Repo, commitSHA, branchHead)
	if err != nil {
		slog.Error("tag trigger: reachability check", "app", app.ID, "tag", tag, "err", err)
		res.Status, res.Reason = "error", "could not verify the tag against the branch; redeliver the webhook"
		return res
	}
	if reach != provider.CompareAhead && reach != provider.CompareIdentical {
		reason := fmt.Sprintf("tag %s points at %s which is not reachable from branch %q; merge it first, then push the tag again or redeliver the webhook",
			tag, shortSHA(commitSHA), app.Branch)
		slog.Warn("tag trigger: tag commit not reachable from branch",
			"app", app.ID, "tag", tag, "sha", commitSHA, "branch", app.Branch)
		// No TriggerTag on this record: once the commit is merged, the same
		// tag may legitimately start a run.
		now := time.Now()
		skipped := &store.PipelineRun{
			ID:            uuid.New(),
			ApplicationID: app.ID,
			CommitSHA:     commitSHA,
			CommitMessage: "Tag " + tag,
			Branch:        app.Branch,
			TriggeredBy:   event.AuthorName,
			Status:        "skipped",
			Tag:           tag,
			StartedAt:     &now,
			CompletedAt:   &now,
		}
		if err := h.store.CreatePipelineRun(ctx, skipped); err != nil {
			slog.Error("create skipped run record", "err", err)
		}
		if app.Notifications.OnFailureURL != "" {
			go h.fireUnreachableTagNotification(app.Notifications, app, tag, commitSHA, reason)
		}
		res.Status, res.Reason = "skipped", reason
		return res
	}

	run := &store.PipelineRun{
		ID:            uuid.New(),
		ApplicationID: app.ID,
		CommitSHA:     commitSHA,
		CommitMessage: "Tag " + tag,
		Branch:        app.Branch,
		TriggeredBy:   event.AuthorName,
		Status:        "pending",
		Tag:           tag,
		TriggerTag:    tag,
	}
	if err := h.store.CreatePipelineRun(ctx, run); err != nil {
		if errors.Is(err, store.ErrDuplicateTriggerTag) {
			res.Status, res.Reason = "duplicate", "a run for this tag already exists"
			return res
		}
		slog.Error("create pipeline run", "app", app.ID, "err", err)
		res.Status, res.Reason = "error", "failed to create pipeline run"
		return res
	}
	res.Status, res.RunID = "queued", run.ID.String()
	return res
}

// fireUnreachableTagNotification posts a JSON webhook when a pushed tag was
// rejected because its commit is not reachable from the application's branch.
func (h *Handler) fireUnreachableTagNotification(n store.NotificationConfig, app *store.Application, tag, sha, reason string) {
	payload := map[string]any{
		"event":          "tag.unreachable",
		"application_id": app.ID.String(),
		"branch":         app.Branch,
		"tag":            tag,
		"commit_sha":     sha,
		"reason":         reason,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		n.OnFailureURL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("unreachable tag notification: build request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("unreachable tag notification: request failed", "url", n.OnFailureURL, "err", err)
		return
	}
	defer resp.Body.Close()
	slog.Info("unreachable tag notification: sent", "url", n.OnFailureURL, "status", resp.StatusCode)
}

// ── Commit lineage ────────────────────────────────────────────────────────────

// headRecoveryInstructions tells an operator how to unblock an application
// after a history rewrite. Included in the blocked reason shown in the UI,
// API, and CLI.
const headRecoveryInstructions = "New pipeline runs are paused for this application. " +
	"To recover: (1) confirm the branch rewrite was intentional with whoever pushed it, " +
	"(2) verify existing release tags still point at commits reachable from the branch, " +
	"(3) accept the current branch head on the application page (or POST /api/applications/{id}/head/accept)."

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// processLineage validates that the pushed commit chains onto the branch head
// bifrost last saw, and advances the stored head when it does.
//
// Every non-force push to a branch fast-forwards it, so the payload's 'before'
// equals the previously seen head regardless of how the commit landed (direct
// push, merge commit, squash merge, or rebase merge). The only events that
// break the chain are history rewrites — exactly what must block releases.
//
// Returns proceed=false with a result status and reason when no pipeline run
// should be created for this application.
func (h *Handler) processLineage(ctx context.Context, app *store.Application, prov provider.Provider, event provider.PushEvent) (proceed bool, status, reason string) {
	if app.HeadState == store.HeadStateBlocked {
		slog.Warn("webhook ignored: application blocked pending manual head reconciliation",
			"app", app.ID, "sha", event.CommitSHA)
		return false, "blocked", app.BlockedReason
	}

	if event.CommitSHA == provider.ZeroSHA {
		h.blockApp(ctx, app, event, fmt.Sprintf(
			"The tracked branch %q was deleted (previous head %s).",
			app.Branch, shortSHA(event.BeforeSHA)))
		return false, "blocked", "tracked branch deleted"
	}

	if event.Forced {
		h.blockApp(ctx, app, event, fmt.Sprintf(
			"Force push detected on branch %q: head moved from %s to %s non-fast-forward.",
			app.Branch, shortSHA(event.BeforeSHA), shortSHA(event.CommitSHA)))
		return false, "blocked", "force push detected"
	}

	// First contact (new application or pre-lineage data): adopt the pushed
	// head as the baseline.
	if app.LastKnownSHA == "" {
		if _, err := h.store.AdvanceApplicationHead(ctx, app.ID, "", event.CommitSHA); err != nil {
			// Not fatal: the run proceeds and the next webhook re-adopts.
			slog.Error("lineage: failed to adopt initial head", "app", app.ID, "err", err)
		}
		return true, "", ""
	}

	// Normal case: the push chains directly onto the known head.
	if event.BeforeSHA == app.LastKnownSHA {
		return h.advanceHead(ctx, app, event)
	}

	// The push does not chain onto our head. Ask the provider how the pushed
	// head relates to the head we know.
	cmp, err := prov.CompareCommits(ctx, app.Owner, app.Repo, app.LastKnownSHA, event.CommitSHA)
	if errors.Is(err, provider.ErrNotFound) {
		h.blockApp(ctx, app, event, fmt.Sprintf(
			"History rewrite detected on branch %q: the previously known head %s no longer exists in the repository (pushed head %s).",
			app.Branch, shortSHA(app.LastKnownSHA), shortSHA(event.CommitSHA)))
		return false, "blocked", "previous head unreachable"
	}
	if err != nil {
		slog.Error("lineage: compare failed", "app", app.ID,
			"base", app.LastKnownSHA, "head", event.CommitSHA, "err", err)
		return false, "error", "could not verify commit lineage against the provider; redeliver the webhook"
	}

	switch cmp {
	case provider.CompareAhead:
		// The pushed head fast-forwards our known head but skipped over
		// deliveries we never saw (missed webhooks, e.g. bifrost downtime).
		// Sync to the new head; the release itself is complete because semver
		// and changelog derive their input from commits since the last tag.
		slog.Info("lineage: head ahead of known head, syncing over missed pushes",
			"app", app.ID, "from", shortSHA(app.LastKnownSHA), "to", shortSHA(event.CommitSHA),
			"claimed_before", shortSHA(event.BeforeSHA))
		return h.advanceHead(ctx, app, event)
	case provider.CompareIdentical, provider.CompareBehind:
		// Stale, duplicate, or out-of-order delivery: a newer head is already
		// tracked. Nothing to release.
		slog.Info("lineage: stale delivery ignored", "app", app.ID,
			"sha", shortSHA(event.CommitSHA), "known_head", shortSHA(app.LastKnownSHA))
		return false, "stale", ""
	default: // diverged
		h.blockApp(ctx, app, event, fmt.Sprintf(
			"Force push detected on branch %q: pushed head %s does not contain the previously known head %s.",
			app.Branch, shortSHA(event.CommitSHA), shortSHA(app.LastKnownSHA)))
		return false, "blocked", "history diverged"
	}
}

// advanceHead CAS-advances the stored branch head to the pushed head. Returns
// proceed=false with a result status when a concurrent delivery advanced it
// first or the head could not be recorded.
func (h *Handler) advanceHead(ctx context.Context, app *store.Application, event provider.PushEvent) (proceed bool, status, reason string) {
	ok, err := h.store.AdvanceApplicationHead(ctx, app.ID, app.LastKnownSHA, event.CommitSHA)
	if err != nil {
		slog.Error("lineage: failed to advance head", "app", app.ID, "err", err)
		return false, "error", "failed to record branch head; redeliver the webhook"
	}
	if !ok {
		// A concurrent delivery moved the head first; its run covers this
		// push's commits (or will supersede them).
		slog.Info("lineage: delivery superseded by a concurrent one", "app", app.ID,
			"sha", shortSHA(event.CommitSHA))
		return false, "superseded", ""
	}
	return true, "", ""
}

// blockApp marks the application as blocked with a reason and recovery
// instructions, cancels queued runs (their commits may no longer exist on the
// rewritten branch), records a 'blocked' run for timeline visibility, and
// fires the failure notification webhook.
func (h *Handler) blockApp(ctx context.Context, app *store.Application, event provider.PushEvent, detail string) {
	reason := detail + " " + headRecoveryInstructions
	slog.Warn("blocking application", "app", app.ID, "reason", detail)

	if err := h.store.BlockApplication(ctx, app.ID, reason); err != nil {
		slog.Error("failed to block application", "app", app.ID, "err", err)
		return
	}

	cancelled, err := h.store.CancelPendingRuns(ctx, app.ID)
	if err != nil {
		slog.Error("failed to cancel pending runs of blocked application", "app", app.ID, "err", err)
	}
	for _, id := range cancelled {
		slog.Info("cancelled pending run of blocked application", "run_id", id, "app", app.ID)
		h.broker.Notify(id)
	}

	now := time.Now()
	blocked := &store.PipelineRun{
		ID:            uuid.New(),
		ApplicationID: app.ID,
		CommitSHA:     event.CommitSHA,
		ParentSHA:     event.BeforeSHA,
		CommitMessage: event.CommitMsg,
		Branch:        event.Branch,
		TriggeredBy:   event.AuthorName,
		Status:        "blocked",
		StartedAt:     &now,
		CompletedAt:   &now,
	}
	if err := h.store.CreatePipelineRun(ctx, blocked); err != nil {
		slog.Error("failed to record blocked run", "app", app.ID, "err", err)
	}

	if app.Notifications.OnFailureURL != "" {
		go h.fireBlockedNotification(app.Notifications, app, event, reason)
	}
}

// fireBlockedNotification posts a JSON webhook when an application is blocked,
// so operators find out without watching the dashboard.
func (h *Handler) fireBlockedNotification(n store.NotificationConfig, app *store.Application, event provider.PushEvent, reason string) {
	payload := map[string]any{
		"event":          "application.blocked",
		"application_id": app.ID.String(),
		"branch":         app.Branch,
		"commit_sha":     event.CommitSHA,
		"before_sha":     event.BeforeSHA,
		"reason":         reason,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		n.OnFailureURL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("blocked notification: build request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("blocked notification: request failed", "url", n.OnFailureURL, "err", err)
		return
	}
	defer resp.Body.Close()
	slog.Info("blocked notification: sent", "url", n.OnFailureURL, "status", resp.StatusCode)
}

// AcceptApplicationHead re-baselines the application's tracked branch head at
// the provider's current value and unblocks new pipeline runs. It is the
// manual recovery step after a force push. Optional body:
// {"trigger_run": true} additionally enqueues a run for the accepted head.
func (h *Handler) AcceptApplicationHead(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	app, err := h.store.GetApplication(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	prov, ok := h.providers[app.Provider]
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("provider %q not configured", app.Provider))
		return
	}

	// Never trust a stale webhook for the new baseline: ask the provider for
	// the live branch head.
	head, err := prov.GetBranchHead(r.Context(), app.Owner, app.Repo, app.Branch)
	if errors.Is(err, provider.ErrNotFound) {
		writeError(w, http.StatusConflict, fmt.Sprintf("branch %q not found on the provider", app.Branch))
		return
	}
	if err != nil {
		slog.Error("accept head: get branch head", "app", app.ID, "err", err)
		writeError(w, http.StatusBadGateway, "failed to fetch current branch head from provider")
		return
	}

	if err := h.store.AcceptApplicationHead(r.Context(), app.ID, head); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to accept branch head")
		return
	}

	resolvedBy := "api-key"
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
		resolvedBy = claims.Email
	}
	slog.Info("application head accepted", "app", app.ID, "head", head, "by", resolvedBy)

	var body struct {
		TriggerRun bool `json:"trigger_run"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck

	resp := map[string]string{"head": head, "head_state": store.HeadStateOK}
	if body.TriggerRun && app.TriggerType == store.TriggerTag {
		writeError(w, http.StatusUnprocessableEntity,
			"head accepted, but tag-triggered applications cannot start a run from a branch head; push a tag instead")
		return
	}
	if body.TriggerRun {
		run := &store.PipelineRun{
			ID:            uuid.New(),
			ApplicationID: app.ID,
			CommitSHA:     head,
			Branch:        app.Branch,
			TriggeredBy:   resolvedBy,
			Status:        "pending",
		}
		if err := h.store.CreatePipelineRun(r.Context(), run); err != nil {
			slog.Error("accept head: create run", "app", app.ID, "err", err)
			writeError(w, http.StatusInternalServerError, "head accepted but failed to create pipeline run")
			return
		}
		select {
		case h.pollNow <- struct{}{}:
		default:
		}
		resp["run_id"] = run.ID.String()
	}

	writeJSON(w, http.StatusOK, resp)
}

// requireAppAccess returns false and writes a 403 when the request carries a
// user JWT and that user does not have access to the application.
// API-key requests (no JWT claims in context) always pass.
func (h *Handler) requireAppAccess(w http.ResponseWriter, r *http.Request, appID uuid.UUID) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		return true // API key — full access
	}
	if claims.IsAdmin {
		return true // admins always have access to all applications
	}
	allowed, err := h.store.CanUserAccessApplication(r.Context(), claims.UserID, appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "access check failed")
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// ── Applications ──────────────────────────────────────────────────────────────

// applicationWithLastRun decorates an application with its most recent
// pipeline run for list views. LastRun is nil when the app has never run.
type applicationWithLastRun struct {
	*store.Application
	LastRun *store.PipelineRun
}

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	var apps []*store.Application
	var err error
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
		apps, err = h.store.ListApplicationsForUser(r.Context(), claims.UserID)
	} else {
		apps, err = h.store.ListApplications(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list applications")
		return
	}
	latest, err := h.store.ListLatestRuns(r.Context())
	if err != nil {
		// The list itself is still useful without run info; degrade instead of failing.
		slog.Warn("list latest runs", "err", err)
		latest = map[uuid.UUID]*store.PipelineRun{}
	}
	out := make([]applicationWithLastRun, 0, len(apps))
	for _, app := range apps {
		app.WebhookSecret = ""
		out = append(out, applicationWithLastRun{Application: app, LastRun: latest[app.ID]})
	}
	writeJSON(w, http.StatusOK, out)
}

// ListProviders returns the IDs of git providers configured on this server,
// so clients can avoid offering providers that would fail at webhook time.
func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	ids := make([]string, 0, len(h.providers))
	for id := range h.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	writeJSON(w, http.StatusOK, map[string][]string{"providers": ids})
}

// presatisfiedSteps lists step requirements met outside the pipeline:
// tag-triggered apps get their release tag from the pushed tag, satisfying
// "semver" without the step.
func presatisfiedSteps(a *store.Application) []string {
	if a.TriggerType == store.TriggerTag {
		return []string{"semver"}
	}
	return nil
}

// validateApplication normalises an empty trigger type to push and checks
// the trigger configuration invariants.
func validateApplication(a *store.Application) error {
	if a.TriggerType == "" {
		a.TriggerType = store.TriggerPush
	}
	switch a.TriggerType {
	case store.TriggerPush:
		if a.TagPattern != "" {
			return errors.New("tag_pattern only applies to tag-triggered applications")
		}
	case store.TriggerTag:
		if a.TagPattern == "" {
			return errors.New("tag-triggered applications require a tag_pattern (e.g. \"v*\")")
		}
		if a.Branch == "" {
			return errors.New("tag-triggered applications require a branch: pushed tags must point at commits reachable from it")
		}
		for _, s := range a.PipelineSteps {
			if s.Type == "semver" || s.Type == "tag" {
				return fmt.Errorf("step %q does not apply to tag-triggered applications: the pushed tag already provides the version", s.Type)
			}
		}
	default:
		return fmt.Errorf("unknown trigger type %q (expected %q or %q)", a.TriggerType, store.TriggerPush, store.TriggerTag)
	}
	return nil
}

func (h *Handler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var a store.Application
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateApplication(&a); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.registry.Build(a.PipelineSteps, presatisfiedSteps(&a)...); err != nil {
		writeError(w, http.StatusBadRequest, "invalid pipeline: "+err.Error())
		return
	}
	if a.WebhookSecret != "" {
		enc, err := h.encryptSecret(a.WebhookSecret)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt webhook secret")
			return
		}
		a.WebhookSecret = enc
	}
	if err := h.store.CreateApplication(r.Context(), &a); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	a.WebhookSecret = ""
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	a, err := h.store.GetApplication(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	a.WebhookSecret = ""
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	var a store.Application
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateApplication(&a); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.registry.Build(a.PipelineSteps, presatisfiedSteps(&a)...); err != nil {
		writeError(w, http.StatusBadRequest, "invalid pipeline: "+err.Error())
		return
	}
	a.ID = id
	if a.WebhookSecret != "" {
		enc, err := h.encryptSecret(a.WebhookSecret)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt webhook secret")
			return
		}
		a.WebhookSecret = enc
	}
	if err := h.store.UpdateApplication(r.Context(), &a); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update application")
		return
	}
	a.WebhookSecret = ""
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) DeleteApplication(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	if err := h.store.DeleteApplication(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete application")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// InstallWebhook creates or updates a webhook on the application's repository using
// the stored webhook secret. Requires PUBLIC_URL to be configured so the provider
// knows where to send events.
func (h *Handler) InstallWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	app, err := h.store.GetApplication(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	if h.publicURL == "" {
		writeError(w, http.StatusUnprocessableEntity, "PUBLIC_URL is not configured; set it to the externally reachable URL of this Bifrost instance")
		return
	}
	prov, ok := h.providers[app.Provider]
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "provider not configured: "+app.Provider)
		return
	}
	if app.WebhookSecret == "" {
		writeError(w, http.StatusUnprocessableEntity, "application has no webhook secret")
		return
	}
	plainSecret, err := h.decryptSecret(app.WebhookSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt webhook secret")
		return
	}
	webhookURL := h.publicURL + "/webhooks/" + app.Provider
	// "create" carries tag creation on Gitea/Forgejo.
	if err := prov.InstallWebhook(r.Context(), app.Owner, app.Repo, webhookURL, plainSecret, []string{"push", "create", "workflow_run"}); err != nil {
		writeError(w, http.StatusBadGateway, "failed to install webhook: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"webhook_url": webhookURL})
}

// ── Application group access ──────────────────────────────────────────────────

func (h *Handler) ListApplicationGroups(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	groups, err := h.store.ListApplicationGroups(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *Handler) GrantGroupAccess(w http.ResponseWriter, r *http.Request) {
	appID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAdminOrAppAccess(w, r, appID) {
		return
	}
	groupID, err := parseUUID(chi.URLParam(r, "groupId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	if err := h.store.GrantGroupAccess(r.Context(), appID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to grant access")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RevokeGroupAccess(w http.ResponseWriter, r *http.Request) {
	appID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAdminOrAppAccess(w, r, appID) {
		return
	}
	groupID, err := parseUUID(chi.URLParam(r, "groupId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	if err := h.store.RevokeGroupAccess(r.Context(), appID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke access")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Runs ──────────────────────────────────────────────────────────────────────

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	if !h.requireAppAccess(w, r, id) {
		return
	}
	limit, offset := 20, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	filter := store.RunFilter{
		Status: r.URL.Query().Get("status"),
		Branch: r.URL.Query().Get("branch"),
	}
	runs, err := h.store.ListPipelineRuns(r.Context(), id, limit, offset, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := h.store.GetPipelineRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if !h.requireAppAccess(w, r, run.ApplicationID) {
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// OverrideStep marks a failed step as manually overridden and resumes the run
// from the step after it. A failed step never continues on its own (e.g. a
// dispatched workflow that concluded 'failure'); a human must take
// responsibility, and the mandatory reason plus their identity are recorded on
// the step result for the audit trail.
func (h *Handler) OverrideStep(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	stepIndex, err := strconv.Atoi(chi.URLParam(r, "stepIndex"))
	if err != nil || stepIndex < 0 {
		writeError(w, http.StatusBadRequest, "invalid step index")
		return
	}

	var body struct {
		Reason string `json:"reason"`
		By     string `json:"by"` // used for API-key requests; JWT identity wins
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "a reason is required to override a failed step")
		return
	}
	by := strings.TrimSpace(body.By)
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
		by = claims.Email
	}
	if by == "" {
		by = "api-key"
	}

	run, err := h.store.GetPipelineRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if !h.requireAppAccess(w, r, run.ApplicationID) {
		return
	}
	if run.Status != "failed" {
		writeError(w, http.StatusConflict, "only failed runs can have a step overridden")
		return
	}

	application, err := h.store.GetApplication(r.Context(), run.ApplicationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load application")
		return
	}
	if _, ok := h.providers[application.Provider]; !ok {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("provider %q not configured", application.Provider))
		return
	}

	ok, err := h.store.OverrideStepResult(r.Context(), runID, stepIndex, by, reason)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to override step")
		return
	}
	if !ok {
		writeError(w, http.StatusConflict, "step is not in a failed state")
		return
	}

	// Reset the steps after the overridden one so the run resumes there.
	if err := h.store.ResetStepResultsFrom(r.Context(), runID, stepIndex+1); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset step results")
		return
	}
	if err := h.store.DeleteApprovalRequestsFrom(r.Context(), runID, stepIndex+1); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset approval requests")
		return
	}
	if err := h.store.ResetRunToPending(r.Context(), runID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset run")
		return
	}
	select {
	case h.pollNow <- struct{}{}:
	default:
	}

	slog.Info("step overridden", "run_id", runID, "step_index", stepIndex, "by", by, "reason", reason)
	h.broker.Notify(runID)
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID.String()})
}

func (h *Handler) RetryStep(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	stepIndex, err := strconv.Atoi(chi.URLParam(r, "stepIndex"))
	if err != nil || stepIndex < 0 {
		writeError(w, http.StatusBadRequest, "invalid step index")
		return
	}

	run, err := h.store.GetPipelineRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if run.Status != "failed" && run.Status != "cancelled" {
		writeError(w, http.StatusConflict, "only failed or cancelled runs can have steps retried")
		return
	}

	steps, err := h.store.ListStepResults(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load step results")
		return
	}
	var target *store.StepResult
	for _, s := range steps {
		if s.StepIndex == stepIndex {
			target = s
			break
		}
	}
	if target == nil || (target.Status != "failed" && target.Status != "cancelled") {
		writeError(w, http.StatusConflict, "step is not in a failed or cancelled state")
		return
	}

	application, err := h.store.GetApplication(r.Context(), run.ApplicationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load application")
		return
	}
	if _, ok := h.providers[application.Provider]; !ok {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("provider %q not configured", application.Provider))
		return
	}

	// Reset step results to pending so they stay visible in the UI while the
	// run is re-queued. ExecuteFrom will delete and recreate them when it runs.
	// Approval requests are deleted so the step creates a fresh one on retry.
	if err := h.store.ResetStepResultsFrom(r.Context(), runID, stepIndex); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset step results")
		return
	}
	if err := h.store.DeleteApprovalRequestsFrom(r.Context(), runID, stepIndex); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset approval requests")
		return
	}
	if err := h.store.ResetRunToPending(r.Context(), runID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset run")
		return
	}
	select {
	case h.pollNow <- struct{}{}:
	default:
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID.String()})
}

// CancelRun cancels a pending or running pipeline run.
// For pending runs the status is updated directly (the poller has not claimed it yet).
// For running runs the goroutine's context is cancelled; the pipeline marks it cancelled.
func (h *Handler) CancelRun(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}

	run, err := h.store.GetPipelineRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if !h.requireAppAccess(w, r, run.ApplicationID) {
		return
	}

	switch run.Status {
	case "pending":
		if err := h.store.CancelRun(r.Context(), runID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel run")
			return
		}
	case "running":
		if !h.cancelRun(runID) {
			// Race: goroutine finished between the status check and here.
			// Attempt a direct DB cancel in case it somehow stayed running.
			_ = h.store.CancelRun(r.Context(), runID)
		}
	default:
		writeError(w, http.StatusConflict, fmt.Sprintf("run is %s and cannot be cancelled", run.Status))
		return
	}

	h.broker.Notify(runID)
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID.String()})
}

// ── Approvals ─────────────────────────────────────────────────────────────────

func (h *Handler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	reqs, err := h.store.ListApprovalRequests(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list approvals")
		return
	}
	writeJSON(w, http.StatusOK, reqs)
}

func (h *Handler) resolveApproval(w http.ResponseWriter, r *http.Request, status string) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	stepIndex, err := strconv.Atoi(chi.URLParam(r, "stepIndex"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step index")
		return
	}

	var body struct {
		By string `json:"by"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck

	req, err := h.store.GetPendingApproval(r.Context(), runID, stepIndex)
	if err != nil {
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}
	if req.Status != "pending" {
		writeError(w, http.StatusConflict, fmt.Sprintf("approval already %s", req.Status))
		return
	}
	if err := h.store.ResolveApprovalRequest(r.Context(), req.ID, status, body.By); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve approval")
		return
	}

	// Wake the waiting approval step goroutine immediately.
	if ch, ok := h.approvalWakers.Load(runID); ok {
		select {
		case ch.(chan struct{}) <- struct{}{}:
		default:
		}
	}
	// Notify the approved run's SSE subscribers immediately.
	h.broker.Notify(runID)

	// When approving, supersede any older runs waiting at the same step.
	if status == "approved" {
		run, err := h.store.GetPipelineRun(r.Context(), runID)
		if err == nil {
			app, err := h.store.GetApplication(r.Context(), run.ApplicationID)
			if err == nil {
				supersededRunIDs, _ := h.store.SupersedeOlderApprovals(r.Context(), app.ID, req.StepIndex, runID)
				for _, sid := range supersededRunIDs {
					slog.Info("run superseded", "run_id", sid, "by", runID)
					h.broker.Notify(sid)
				}
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ApproveStep(w http.ResponseWriter, r *http.Request) {
	h.resolveApproval(w, r, "approved")
}

func (h *Handler) RejectStep(w http.ResponseWriter, r *http.Request) {
	h.resolveApproval(w, r, "rejected")
}

// ── Groups ────────────────────────────────────────────────────────────────────

func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	groups, err := h.store.ListGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var g store.Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.store.CreateGroup(r.Context(), &g); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	g := &store.Group{ID: id, Name: req.Name}
	if err := h.store.UpdateGroup(r.Context(), g); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename group")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	if err := h.store.DeleteGroup(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	members, err := h.store.ListGroupMembers(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *Handler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	userID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.store.AddUserToGroup(r.Context(), userID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	userID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.store.RemoveUserFromGroup(r.Context(), userID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Setup ─────────────────────────────────────────────────────────────────────

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetDashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load dashboard stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// GetHealth returns 200 OK for liveness/readiness probes.
func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetSetupStatus returns whether the instance still needs its first user created.
// This endpoint is public — it is used by the frontend to gate the setup wizard.
func (h *Handler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needs_setup": len(users) == 0})
}

// Setup creates the first admin user. Returns 409 if setup has already been completed.
func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}
	if len(users) > 0 {
		writeError(w, http.StatusConflict, "setup already complete")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	u := &store.User{Email: req.Email, PasswordHash: string(hash), IsAdmin: true}
	if err := h.store.CreateUser(r.Context(), u); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"email": u.Email})
}

// ── Auth ──────────────────────────────────────────────────────────────────────

// sessionTTL bounds both the JWT expiry and the session cookie lifetime.
const sessionTTL = 24 * time.Hour

// secureCookies reports whether session cookies should carry the Secure
// flag, derived from the instance being served over HTTPS.
func (h *Handler) secureCookies() bool {
	return strings.HasPrefix(h.publicURL, "https://")
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	emailKey, ipKey := loginRateLimitKeys(r, req.Email)
	if remaining, locked := h.loginLimiter.locked(emailKey); locked {
		writeRateLimited(w, remaining)
		return
	}
	if remaining, locked := h.loginLimiter.locked(ipKey); locked {
		writeRateLimited(w, remaining)
		return
	}

	user, err := h.store.GetUserForAuth(r.Context(), req.Email)
	if err != nil {
		h.loginLimiter.recordFailure(emailKey)
		h.loginLimiter.recordFailure(ipKey)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.loginLimiter.recordFailure(emailKey)
		h.loginLimiter.recordFailure(ipKey)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	h.loginLimiter.reset(emailKey)
	h.loginLimiter.reset(ipKey)

	token, err := auth.GenerateToken(user.ID, user.Email, user.IsAdmin, h.jwtSecret, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Browsers authenticate via this httpOnly cookie; the token in the body
	// is for non-browser clients (bifrost-cli) that send it as a Bearer header.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secureCookies(),
	})
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// Logout clears the session cookie. The JWT itself remains valid until it
// expires; this only removes it from the browser.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secureCookies(),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  claims.UserID,
		"email":    claims.Email,
		"is_admin": claims.IsAdmin,
	})
}

// requireAdmin returns true if the request comes from an admin user or the
// static API key (which is always treated as admin). Returns false and writes
// a 403 response when the check fails.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		return true // API key — full access
	}
	if !claims.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// requireAdminOrAppAccess returns true when the request is from an admin, the
// static API key, or a user who already has access to the application.
func (h *Handler) requireAdminOrAppAccess(w http.ResponseWriter, r *http.Request, appID uuid.UUID) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		return true // API key — full access
	}
	if claims.IsAdmin {
		return true
	}
	allowed, err := h.store.CanUserAccessApplication(r.Context(), claims.UserID, appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "access check failed")
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// ── Users ─────────────────────────────────────────────────────────────────────

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	u := &store.User{Email: req.Email, PasswordHash: string(hash)}
	if err := h.store.CreateUser(r.Context(), u); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// DeleteUser removes a user. Refuses to delete the caller's own account (this
// app has no account-recovery flow, so a self-delete would be unrecoverable)
// and refuses to delete the last remaining admin.
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if claims, ok := auth.ClaimsFromContext(r.Context()); ok && claims.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	var target *store.User
	for _, u := range users {
		if u.ID == id {
			target = u
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.IsAdmin {
		admins, err := h.store.CountAdmins(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to count admins")
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusBadRequest, "cannot delete the last admin")
			return
		}
	}

	if err := h.store.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ResetUserPassword lets an admin set another user's password directly, without
// knowing their current one.
func (h *Handler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.store.UpdateUserPassword(r.Context(), id, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetUserAdmin grants or revokes admin rights on a user. Refuses to demote the
// last remaining admin so the instance can never end up without one.
func (h *Handler) SetUserAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	var target *store.User
	for _, u := range users {
		if u.ID == id {
			target = u
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.IsAdmin == req.IsAdmin {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !req.IsAdmin {
		admins, err := h.store.CountAdmins(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to count admins")
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusBadRequest, "cannot demote the last admin")
			return
		}
	}

	if err := h.store.SetUserAdmin(r.Context(), id, req.IsAdmin); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ChangePassword lets an authenticated user change their own password after
// verifying their current one. Not available to static-API-key requests since
// those aren't tied to a specific user account.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusBadRequest, "not available for API key requests")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	user, err := h.store.GetUserForAuth(r.Context(), claims.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// 400, not 401: the bearer token is valid, only the submitted password is
	// wrong. Clients treat 401 as an expired session.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.store.UpdateUserPassword(r.Context(), user.ID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWorkflowRunCompleted updates the step result that triggered the workflow
// and notifies any SSE subscribers so the frontend reflects the conclusion.
func (h *Handler) handleWorkflowRunCompleted(ctx context.Context, wre provider.WorkflowRunEvent) {
	step, err := h.store.GetStepResultByExternalRunID(ctx, wre.RunID)
	if err != nil {
		slog.Debug("workflow_run completed: no matching step result", "run_id", wre.RunID)
		return
	}

	slog.Info("workflow_run completed",
		"external_run_id", wre.RunID,
		"workflow", wre.Name,
		"conclusion", wre.Conclusion,
		"step_result_id", step.ID,
	)

	step.Output = fmt.Sprintf("Workflow %q completed with conclusion: %s", wre.Name, wre.Conclusion)
	if wre.Conclusion != "success" && wre.Conclusion != "skipped" {
		step.ErrorMessage = fmt.Sprintf("workflow run %d failed: %s", wre.RunID, wre.Conclusion)
	}

	if err := h.store.UpdateStepResult(ctx, step); err != nil {
		slog.Error("workflow_run: update step result", "err", err)
		return
	}

	h.broker.Notify(step.RunID)
}

// ── Run events (SSE) ──────────────────────────────────────────────────────────

// StreamRunEvents streams Server-Sent Events for a single run.
// Clients receive an "update" event whenever run or step state changes,
// and a "ping" keepalive every 15 seconds. The stream stays open until the
// client disconnects or the request context is cancelled.
func (h *Handler) StreamRunEvents(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx/proxy buffering

	rc := http.NewResponseController(w)
	// Disable the server-level write timeout for this long-lived connection.
	rc.SetWriteDeadline(time.Time{}) //nolint:errcheck

	ch, unsub := h.broker.Subscribe(runID)
	defer unsub()

	send := func(event string) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: {}\n\n", event); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	// Send an initial ping so the client knows the connection is established.
	if !send("ping") {
		return
	}

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if !send("update") {
				return
			}
		case <-keepalive.C:
			if !send("ping") {
				return
			}
		}
	}
}

// ── Step results ──────────────────────────────────────────────────────────────

func (h *Handler) ListStepResults(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	results, err := h.store.ListStepResults(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list step results")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	writeError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
