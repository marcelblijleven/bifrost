// Package gitea implements provider.Provider for Gitea and Forgejo.
// Both platforms share the same REST API; only the base URL and provider ID differ.
package gitea

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/marcelblijleven/bifrost/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// Provider implements provider.Provider for a Gitea or Forgejo instance.
type Provider struct {
	id          string // "gitea" or "forgejo"
	instanceURL string // https://gitea.example.com (no trailing slash)
	baseURL     string // https://gitea.example.com/api/v1
	token       string
	http        *http.Client
}

// New creates a Provider for a Gitea or Forgejo instance.
// id should be "gitea" or "forgejo"; instanceURL is the root URL of the instance
// (e.g. "https://gitea.example.com") — /api/v1 is appended automatically.
func New(id, instanceURL, token string) *Provider {
	root := strings.TrimRight(instanceURL, "/")
	return &Provider{
		id:          id,
		instanceURL: root,
		baseURL:     root + "/api/v1",
		token:       token,
		http:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Provider) ID() string { return p.id }

// ── Webhook ───────────────────────────────────────────────────────────────────

// ParseWebhook validates the HMAC-SHA256 signature and parses the push payload.
// Gitea sends X-Gitea-Event; Forgejo sends both X-Forgejo-Event and X-Gitea-Event.
func (p *Provider) ParseWebhook(r *http.Request, secret string) (provider.PushEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return provider.PushEvent{}, fmt.Errorf("read body: %w", err)
	}
	defer r.Body.Close()

	// Validate signature (Forgejo also populates X-Gitea-Signature for compat).
	sig := r.Header.Get("X-Gitea-Signature")
	if sig == "" {
		sig = r.Header.Get("X-Forgejo-Signature")
	}
	if sig == "" {
		return provider.PushEvent{}, fmt.Errorf("missing webhook signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig)) {
		return provider.PushEvent{}, fmt.Errorf("webhook signature mismatch")
	}

	eventType := r.Header.Get("X-Gitea-Event")
	if eventType == "" {
		eventType = r.Header.Get("X-Forgejo-Event")
	}
	// Gitea/Forgejo deliver tag creation as a "create" event, not a push.
	if eventType == "create" {
		return parseCreateEvent(p.id, body)
	}
	if eventType != "push" {
		return provider.PushEvent{}, provider.ErrNotPushEvent
	}

	var payload giteaPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return provider.PushEvent{}, fmt.Errorf("parse push payload: %w", err)
	}

	// Some Gitea/Forgejo versions also mirror tag pushes as push events.
	if tag := strings.TrimPrefix(payload.Ref, "refs/tags/"); tag != payload.Ref {
		return provider.PushEvent{
			ProviderID: p.id,
			RepoOwner:  payload.Repository.Owner.Login,
			RepoName:   payload.Repository.Name,
			TagName:    tag,
			CommitSHA:  payload.After,
			BeforeSHA:  payload.Before,
		}, nil
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	if branch == payload.Ref {
		// Neither a branch nor a tag push.
		return provider.PushEvent{}, provider.ErrNotPushEvent
	}

	var commitMsg, authorName, authorEmail string
	if payload.HeadCommit != nil {
		commitMsg = payload.HeadCommit.Message
		authorName = payload.HeadCommit.Author.Name
		authorEmail = payload.HeadCommit.Author.Email
	} else if len(payload.Commits) > 0 {
		last := payload.Commits[len(payload.Commits)-1]
		commitMsg = last.Message
		authorName = last.Author.Name
		authorEmail = last.Author.Email
	}

	seen := make(map[string]struct{})
	var changedFiles []string
	for _, c := range payload.Commits {
		for _, f := range append(c.Added, c.Modified...) {
			if _, ok := seen[f]; !ok {
				seen[f] = struct{}{}
				changedFiles = append(changedFiles, f)
			}
		}
	}

	return provider.PushEvent{
		ProviderID: p.id,
		RepoOwner:  payload.Repository.Owner.Login,
		RepoName:   payload.Repository.Name,
		Branch:     branch,
		CommitSHA:  payload.After,
		BeforeSHA:  payload.Before,
		// Gitea/Forgejo push payloads carry no force-push flag; force pushes
		// are detected via the ancestry check on BeforeSHA mismatch instead.
		CommitMsg:    commitMsg,
		AuthorName:   authorName,
		AuthorEmail:  authorEmail,
		ChangedFiles: changedFiles,
	}, nil
}

// compareCommitCount returns how many commits are reachable from head but not
// from base.
func (p *Provider) compareCommitCount(ctx context.Context, owner, repo, base, head string) (int, error) {
	var out struct {
		TotalCommits int               `json:"total_commits"`
		Commits      []json.RawMessage `json:"commits"`
	}
	path := fmt.Sprintf("/repos/%s/%s/compare/%s...%s", owner, repo, base, head)
	if err := p.get(ctx, path, &out); err != nil {
		return 0, err
	}
	if out.TotalCommits > 0 {
		return out.TotalCommits, nil
	}
	return len(out.Commits), nil
}

// CompareCommits reports how head relates to base. Gitea's compare API only
// returns the commit list, so the relationship is derived by comparing both
// directions.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (provider.CompareStatus, error) {
	forward, err := p.compareCommitCount(ctx, owner, repo, base, head)
	if err != nil {
		return "", fmt.Errorf("compare %s...%s: %w", base, head, err)
	}
	backward, err := p.compareCommitCount(ctx, owner, repo, head, base)
	if err != nil {
		return "", fmt.Errorf("compare %s...%s: %w", head, base, err)
	}
	switch {
	case forward == 0 && backward == 0:
		return provider.CompareIdentical, nil
	case forward > 0 && backward == 0:
		return provider.CompareAhead, nil
	case forward == 0 && backward > 0:
		return provider.CompareBehind, nil
	default:
		return provider.CompareDiverged, nil
	}
}

// GetBranchHead returns the current head commit SHA of the branch.
func (p *Provider) GetBranchHead(ctx context.Context, owner, repo, branch string) (string, error) {
	var b struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/branches/%s", owner, repo, branch), &b); err != nil {
		return "", fmt.Errorf("get branch %s: %w", branch, err)
	}
	return b.Commit.ID, nil
}

// ParseWorkflowRun decodes a workflow_run webhook payload.
func (p *Provider) ParseWorkflowRun(eventType string, payload []byte) (provider.WorkflowRunEvent, error) {
	if eventType != "workflow_run" {
		return provider.WorkflowRunEvent{}, provider.ErrNotWorkflowRunEvent
	}
	var raw struct {
		Action      string `json:"action"`
		WorkflowRun struct {
			ID         int64  `json:"id"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Name       string `json:"name"`
		} `json:"workflow_run"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return provider.WorkflowRunEvent{}, provider.ErrNotWorkflowRunEvent
	}
	return provider.WorkflowRunEvent{
		RunID:      raw.WorkflowRun.ID,
		Action:     raw.Action,
		Status:     raw.WorkflowRun.Status,
		Conclusion: raw.WorkflowRun.Conclusion,
		Name:       raw.WorkflowRun.Name,
	}, nil
}

// ── Tags ──────────────────────────────────────────────────────────────────────

func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	var tags []string
	page := 1
	for {
		var page_tags []struct {
			Name string `json:"name"`
		}
		path := fmt.Sprintf("/repos/%s/%s/tags?limit=50&page=%d", owner, repo, page)
		if err := p.get(ctx, path, &page_tags); err != nil {
			return nil, fmt.Errorf("list tags (page %d): %w", page, err)
		}
		for _, t := range page_tags {
			tags = append(tags, t.Name)
		}
		if len(page_tags) < 50 {
			break
		}
		page++
	}
	return tags, nil
}

func (p *Provider) CreateTag(ctx context.Context, owner, repo, tag, sha, message string) error {
	body := map[string]any{
		"tag_name": tag,
		"target":   sha,
		"message":  message,
	}
	return p.post(ctx, fmt.Sprintf("/repos/%s/%s/tags", owner, repo), body, nil)
}

// GetTagCommitSHA returns the commit SHA the tag points at.
// Gitea's tag API exposes the target commit directly.
func (p *Provider) GetTagCommitSHA(ctx context.Context, owner, repo, tag string) (string, error) {
	var t struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/tags/%s", owner, repo, tag), &t); err != nil {
		return "", fmt.Errorf("get tag %s: %w", tag, err)
	}
	return t.Commit.SHA, nil
}

// ── Commits ───────────────────────────────────────────────────────────────────

func (p *Provider) ListCommitsSince(ctx context.Context, owner, repo, base, head string) ([]provider.Commit, error) {
	if base == "" {
		var commits []giteaCommit
		if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/commits?sha=%s&limit=100", owner, repo, head), &commits); err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}
		return convertCommits(commits), nil
	}

	// Gitea compare endpoint: /repos/{owner}/{repo}/compare/{base}...{head}
	var compare struct {
		Commits []giteaCommit `json:"commits"`
	}
	path := fmt.Sprintf("/repos/%s/%s/compare/%s...%s", owner, repo, base, head)
	if err := p.get(ctx, path, &compare); err != nil {
		// Fall back to full list if compare fails (e.g. base tag not found)
		var commits []giteaCommit
		if err2 := p.get(ctx, fmt.Sprintf("/repos/%s/%s/commits?sha=%s&limit=100", owner, repo, head), &commits); err2 != nil {
			return nil, fmt.Errorf("compare %s...%s: %w", base, head, err)
		}
		return convertCommits(commits), nil
	}
	return convertCommits(compare.Commits), nil
}

// ListCommitFiles returns the paths touched by a single commit.
func (p *Provider) ListCommitFiles(ctx context.Context, owner, repo, sha string) ([]string, error) {
	var commit struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/git/commits/%s", owner, repo, sha), &commit); err != nil {
		return nil, fmt.Errorf("get commit %s: %w", sha, err)
	}
	files := make([]string, 0, len(commit.Files))
	for _, f := range commit.Files {
		files = append(files, f.Filename)
	}
	return files, nil
}

// ── Releases ──────────────────────────────────────────────────────────────────

func (p *Provider) CreateRelease(ctx context.Context, owner, repo, tag, name, body string, draft, prerelease bool) (string, error) {
	req := map[string]any{
		"tag_name":   tag,
		"name":       name,
		"body":       body,
		"draft":      draft,
		"prerelease": prerelease,
	}
	var rel struct {
		HTMLURL string `json:"html_url"`
	}
	if err := p.post(ctx, fmt.Sprintf("/repos/%s/%s/releases", owner, repo), req, &rel); err != nil {
		return "", fmt.Errorf("create release %s: %w", tag, err)
	}
	return rel.HTMLURL, nil
}

// GetReleaseByTag returns the HTML URL of the release for tag, if one exists.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (string, error) {
	var rel struct {
		HTMLURL string `json:"html_url"`
	}
	if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/releases/tags/%s", owner, repo, tag), &rel); err != nil {
		return "", fmt.Errorf("get release by tag %s: %w", tag, err)
	}
	return rel.HTMLURL, nil
}

// ── Actions ───────────────────────────────────────────────────────────────────

func (p *Provider) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) (int64, string, error) {
	body := map[string]any{"ref": ref, "inputs": inputs}
	before := time.Now()

	// Gitea/Forgejo dispatch returns 204 — no run ID in response.
	if err := p.post(ctx, fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, workflow), body, nil); err != nil {
		return 0, "", fmt.Errorf("dispatch %s: %w", workflow, err)
	}

	// Poll briefly to find the newly created run ID.
	deadline := time.Now().Add(30 * time.Second)
	time.Sleep(2 * time.Second)
	for time.Now().Before(deadline) {
		id, err := p.findRunAfterDispatch(ctx, owner, repo, workflow, before)
		if err == nil {
			runURL := fmt.Sprintf("%s/%s/%s/actions/runs/%d", p.instanceURL, owner, repo, id)
			return id, runURL, nil
		}
		time.Sleep(3 * time.Second)
	}
	return 0, "", fmt.Errorf("dispatch_workflow: could not find new run for %s on %s within 30s", workflow, ref)
}

func (p *Provider) findRunAfterDispatch(ctx context.Context, owner, repo, workflow string, after time.Time) (int64, error) {
	var result struct {
		WorkflowRuns []struct {
			ID        int64     `json:"id"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"workflow_runs"`
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?limit=10", owner, repo)
	if err := p.get(ctx, path, &result); err != nil {
		return 0, err
	}
	for _, r := range result.WorkflowRuns {
		if r.CreatedAt.After(after) {
			return r.ID, nil
		}
	}
	return 0, fmt.Errorf("no matching run found")
}

func (p *Provider) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (provider.WorkflowRun, error) {
	var run struct {
		ID         int64  `json:"id"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID), &run); err != nil {
		return provider.WorkflowRun{}, fmt.Errorf("get workflow run %d: %w", runID, err)
	}
	return provider.WorkflowRun{
		ID:         run.ID,
		Status:     run.Status,
		Conclusion: run.Conclusion,
	}, nil
}

// InstallWebhook creates or updates a webhook on the repository pointing at webhookURL.
// If a webhook for webhookURL already exists it is patched in-place; otherwise a new one is created.
func (p *Provider) InstallWebhook(ctx context.Context, owner, repo, webhookURL, secret string, events []string) error {
	var hooks []struct {
		ID     int64 `json:"id"`
		Config struct {
			URL string `json:"url"`
		} `json:"config"`
	}
	if err := p.get(ctx, fmt.Sprintf("/repos/%s/%s/hooks", owner, repo), &hooks); err != nil {
		return fmt.Errorf("list hooks: %w", err)
	}

	hookBody := map[string]any{
		"config": map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
		"events": events,
		"active": true,
	}

	for _, h := range hooks {
		if h.Config.URL == webhookURL {
			return p.patch(ctx, fmt.Sprintf("/repos/%s/%s/hooks/%d", owner, repo, h.ID), hookBody, nil)
		}
	}

	hookBody["type"] = "gitea"
	return p.post(ctx, fmt.Sprintf("/repos/%s/%s/hooks", owner, repo), hookBody, nil)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (p *Provider) get(ctx context.Context, path string, out any) error {
	return p.do(ctx, http.MethodGet, path, nil, out)
}

func (p *Provider) post(ctx context.Context, path string, body, out any) error {
	return p.do(ctx, http.MethodPost, path, body, out)
}

func (p *Provider) patch(ctx context.Context, path string, body, out any) error {
	return p.do(ctx, http.MethodPatch, path, body, out)
}

func (p *Provider) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+p.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s %s → 404: %w", method, path, provider.ErrNotFound)
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s → %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ── Payload types ─────────────────────────────────────────────────────────────

// parseCreateEvent decodes a Gitea/Forgejo "create" event; only tag creation
// is translated into a PushEvent.
func parseCreateEvent(providerID string, body []byte) (provider.PushEvent, error) {
	var payload struct {
		SHA     string `json:"sha"`
		Ref     string `json:"ref"`
		RefType string `json:"ref_type"`
		Sender  struct {
			Login string `json:"login"`
		} `json:"sender"`
		Repository struct {
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return provider.PushEvent{}, fmt.Errorf("parse create payload: %w", err)
	}
	if payload.RefType != "tag" || payload.Ref == "" {
		return provider.PushEvent{}, provider.ErrNotPushEvent
	}
	return provider.PushEvent{
		ProviderID: providerID,
		RepoOwner:  payload.Repository.Owner.Login,
		RepoName:   payload.Repository.Name,
		TagName:    payload.Ref,
		CommitSHA:  payload.SHA,
		AuthorName: payload.Sender.Login,
	}, nil
}

type giteaPushPayload struct {
	Ref    string `json:"ref"`
	After  string `json:"after"`
	Before string `json:"before"`

	Commits []struct {
		ID       string   `json:"id"`
		Message  string   `json:"message"`
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
		Author   struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`

	HeadCommit *struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"head_commit"`

	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

type giteaCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commit"`
}

func convertCommits(raw []giteaCommit) []provider.Commit {
	out := make([]provider.Commit, 0, len(raw))
	for _, c := range raw {
		msg := c.Commit.Message
		if i := strings.Index(msg, "\n"); i != -1 {
			msg = msg[:i]
		}
		out = append(out, provider.Commit{
			SHA:     c.SHA,
			Message: msg,
			Author:  c.Commit.Author.Name,
		})
	}
	return out
}
