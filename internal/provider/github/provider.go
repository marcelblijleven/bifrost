// Package github implements the provider.Provider interface for GitHub.
package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	gh "github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"

	"github.com/marcelblijleven/bifrost/internal/provider"
)

const apiVersion = "2026-03-10"

var _ provider.Provider = (*Provider)(nil)

// Provider implements provider.Provider for GitHub.
type Provider struct {
	client *gh.Client
}

// NewFromToken creates a GitHub provider authenticated with a personal access token.
// Pass non-empty baseURL/uploadURL for GitHub Enterprise Server or EU-hosted cloud.
func NewFromToken(token, baseURL, uploadURL string) (*Provider, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	if baseURL == "" {
		return &Provider{client: gh.NewClient(tc)}, nil
	}
	client, err := gh.NewEnterpriseClient(baseURL, effectiveUploadURL(baseURL, uploadURL), tc)
	if err != nil {
		return nil, fmt.Errorf("github enterprise client: %w", err)
	}
	return &Provider{client: client}, nil
}

// NewFromApp creates a GitHub provider authenticated as a GitHub App installation.
// privateKey is the PEM-encoded RSA private key for the app.
// Pass non-empty baseURL/uploadURL for GitHub Enterprise Server or EU-hosted cloud.
func NewFromApp(appID, installationID int64, privateKey []byte, baseURL, uploadURL string) (*Provider, error) {
	tr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("github app auth: %w", err)
	}
	if baseURL != "" {
		tr.BaseURL = baseURL
	}
	httpClient := &http.Client{Transport: tr}
	if baseURL == "" {
		return &Provider{client: gh.NewClient(httpClient)}, nil
	}
	client, err := gh.NewEnterpriseClient(baseURL, effectiveUploadURL(baseURL, uploadURL), httpClient)
	if err != nil {
		return nil, fmt.Errorf("github enterprise client: %w", err)
	}
	return &Provider{client: client}, nil
}

// effectiveUploadURL returns uploadURL if set, otherwise derives it from baseURL.
// For GHES (baseURL containing /api/v3/), the upload URL is /api/uploads/ on the same host.
// For all other cases (e.g. EU cloud), fall back to the caller-supplied value or the
// standard github.com upload endpoint.
func effectiveUploadURL(baseURL, uploadURL string) string {
	if uploadURL != "" {
		return uploadURL
	}
	// GHES: https://github.company.com/api/v3/ → https://github.company.com/api/uploads/
	if idx := strings.Index(baseURL, "/api/v3"); idx != -1 {
		return baseURL[:idx] + "/api/uploads/"
	}
	return "https://uploads.github.com/"
}

func (p *Provider) ID() string { return "github" }

// ParseWebhook validates the GitHub webhook signature and parses the payload.
// Returns provider.ErrNotPushEvent when the event type is not a push.
func (p *Provider) ParseWebhook(r *http.Request, secret string) (provider.PushEvent, error) {
	payload, err := gh.ValidatePayload(r, []byte(secret))
	if err != nil {
		return provider.PushEvent{}, fmt.Errorf("validate webhook payload: %w", err)
	}

	eventType := gh.WebHookType(r)
	if eventType != "push" {
		return provider.PushEvent{}, provider.ErrNotPushEvent
	}

	event, err := gh.ParseWebHook(eventType, payload)
	if err != nil {
		return provider.PushEvent{}, fmt.Errorf("parse webhook: %w", err)
	}

	pushEvent, ok := event.(*gh.PushEvent)
	if !ok {
		return provider.PushEvent{}, provider.ErrNotPushEvent
	}

	// A push to refs/tags/... is a tag push: GitHub delivers tag creation and
	// deletion through the same push event type.
	var branch, tagName string
	switch ref := pushEvent.GetRef(); {
	case strings.HasPrefix(ref, "refs/tags/"):
		tagName = strings.TrimPrefix(ref, "refs/tags/")
	default:
		branch = strings.TrimPrefix(ref, "refs/heads/")
	}

	var authorName, authorEmail, commitMsg string
	if hc := pushEvent.GetHeadCommit(); hc != nil {
		commitMsg = hc.GetMessage()
		if author := hc.GetAuthor(); author != nil {
			authorName = author.GetName()
			authorEmail = author.GetEmail()
		}
	}

	// Collect added + modified files across all commits in the push (no extra API call).
	seen := make(map[string]struct{})
	var changedFiles []string
	for _, c := range pushEvent.Commits {
		for _, f := range append(c.Added, c.Modified...) {
			if _, ok := seen[f]; !ok {
				seen[f] = struct{}{}
				changedFiles = append(changedFiles, f)
			}
		}
	}

	return provider.PushEvent{
		ProviderID:   "github",
		RepoOwner:    pushEvent.GetRepo().GetOwner().GetLogin(),
		RepoName:     pushEvent.GetRepo().GetName(),
		Branch:       branch,
		TagName:      tagName,
		CommitSHA:    pushEvent.GetAfter(),
		BeforeSHA:    pushEvent.GetBefore(),
		Forced:       pushEvent.GetForced(),
		CommitMsg:    commitMsg,
		AuthorName:   authorName,
		AuthorEmail:  authorEmail,
		ChangedFiles: changedFiles,
	}, nil
}

// CompareCommits reports how head relates to base using GitHub's compare API.
func (p *Provider) CompareCommits(ctx context.Context, owner, repo, base, head string) (provider.CompareStatus, error) {
	cmp, resp, err := p.client.Repositories.CompareCommits(ctx, owner, repo, base, head, &gh.ListOptions{PerPage: 1})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("compare %s...%s: %w", base, head, provider.ErrNotFound)
		}
		return "", fmt.Errorf("compare %s...%s: %w", base, head, err)
	}
	switch cmp.GetStatus() {
	case "ahead":
		return provider.CompareAhead, nil
	case "behind":
		return provider.CompareBehind, nil
	case "identical":
		return provider.CompareIdentical, nil
	case "diverged":
		return provider.CompareDiverged, nil
	default:
		return "", fmt.Errorf("compare %s...%s: unknown status %q", base, head, cmp.GetStatus())
	}
}

// GetBranchHead returns the current head commit SHA of the branch.
func (p *Provider) GetBranchHead(ctx context.Context, owner, repo, branch string) (string, error) {
	b, resp, err := p.client.Repositories.GetBranch(ctx, owner, repo, branch, 0)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("branch %s: %w", branch, provider.ErrNotFound)
		}
		return "", fmt.Errorf("get branch %s: %w", branch, err)
	}
	return b.GetCommit().GetSHA(), nil
}

// ListTags returns all tag names for the repository, paginating as needed.
func (p *Provider) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	var tags []string
	opts := &gh.ListOptions{PerPage: 100}
	for {
		page, resp, err := p.client.Repositories.ListTags(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}
		for _, t := range page {
			tags = append(tags, t.GetName())
		}
		if resp.NextPage == 0 || len(tags) >= 1000 {
			break
		}
		opts.Page = resp.NextPage
	}
	return tags, nil
}

// CreateTag creates an annotated tag object and its ref in the repository.
func (p *Provider) CreateTag(ctx context.Context, owner, repo, tag, sha, message string) error {
	now := time.Now()
	tagObj, _, err := p.client.Git.CreateTag(ctx, owner, repo, &gh.Tag{
		Tag:     gh.String(tag),
		Message: gh.String(message),
		Object: &gh.GitObject{
			Type: gh.String("commit"),
			SHA:  gh.String(sha),
		},
		Tagger: &gh.CommitAuthor{
			Date:  &gh.Timestamp{Time: now},
			Name:  gh.String("bifrost"),
			Email: gh.String("bifrost@noreply"),
		},
	})
	if err != nil {
		return fmt.Errorf("create tag object %s: %w", tag, err)
	}

	_, _, err = p.client.Git.CreateRef(ctx, owner, repo, &gh.Reference{
		Ref:    gh.String("refs/tags/" + tag),
		Object: &gh.GitObject{SHA: tagObj.SHA},
	})
	if err != nil {
		return fmt.Errorf("create tag ref %s: %w", tag, err)
	}
	return nil
}

// GetTagCommitSHA returns the commit SHA the tag ultimately points at,
// resolving annotated tag objects to their target commit.
func (p *Provider) GetTagCommitSHA(ctx context.Context, owner, repo, tag string) (string, error) {
	ref, resp, err := p.client.Git.GetRef(ctx, owner, repo, "tags/"+tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("tag %s: %w", tag, provider.ErrNotFound)
		}
		return "", fmt.Errorf("get tag ref %s: %w", tag, err)
	}
	obj := ref.GetObject()
	if obj.GetType() != "tag" {
		return obj.GetSHA(), nil // lightweight tag: ref points straight at the commit
	}
	tagObj, _, err := p.client.Git.GetTag(ctx, owner, repo, obj.GetSHA())
	if err != nil {
		return "", fmt.Errorf("get tag object %s: %w", tag, err)
	}
	return tagObj.GetObject().GetSHA(), nil
}

// GetReleaseByTag returns the HTML URL of the release for tag, if one exists.
func (p *Provider) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (string, error) {
	rel, resp, err := p.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("release for tag %s: %w", tag, provider.ErrNotFound)
		}
		return "", fmt.Errorf("get release by tag %s: %w", tag, err)
	}
	return rel.GetHTMLURL(), nil
}

// DispatchWorkflow triggers a workflow_dispatch event and returns the new run ID
// and its HTML URL. Requires API version 2026-03-10 which returns the run ID
// directly in the response body instead of 204 No Content.
func (p *Provider) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) (int64, string, error) {
	inputsAny := make(map[string]any, len(inputs))
	for k, v := range inputs {
		inputsAny[k] = v
	}

	u := fmt.Sprintf("repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, workflow)
	req, err := p.client.NewRequest("POST", u, &gh.CreateWorkflowDispatchEventRequest{
		Ref:    ref,
		Inputs: inputsAny,
	})
	if err != nil {
		return 0, "", fmt.Errorf("build dispatch request for %s: %w", workflow, err)
	}
	req.Header.Set("X-GitHub-Api-Version", apiVersion)

	var result struct {
		WorkflowRunID int64  `json:"workflow_run_id"`
		HTMLURL       string `json:"html_url"`
	}
	if _, err := p.client.Do(ctx, req, &result); err != nil {
		return 0, "", fmt.Errorf("dispatch workflow %s: %w", workflow, err)
	}
	return result.WorkflowRunID, result.HTMLURL, nil
}

// ParseWorkflowRun decodes a pre-validated webhook payload as a workflow_run event.
func (p *Provider) ParseWorkflowRun(eventType string, payload []byte) (provider.WorkflowRunEvent, error) {
	if eventType != "workflow_run" {
		return provider.WorkflowRunEvent{}, provider.ErrNotWorkflowRunEvent
	}
	raw, err := gh.ParseWebHook(eventType, payload)
	if err != nil {
		return provider.WorkflowRunEvent{}, fmt.Errorf("parse workflow_run webhook: %w", err)
	}
	wre, ok := raw.(*gh.WorkflowRunEvent)
	if !ok {
		return provider.WorkflowRunEvent{}, provider.ErrNotWorkflowRunEvent
	}
	return provider.WorkflowRunEvent{
		RunID:      wre.GetWorkflowRun().GetID(),
		Action:     wre.GetAction(),
		Status:     wre.GetWorkflowRun().GetStatus(),
		Conclusion: wre.GetWorkflowRun().GetConclusion(),
		Name:       wre.GetWorkflowRun().GetName(),
	}, nil
}

// GetWorkflowRun fetches the current state of a workflow run by its numeric ID.
func (p *Provider) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (provider.WorkflowRun, error) {
	run, _, err := p.client.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return provider.WorkflowRun{}, fmt.Errorf("get workflow run %d: %w", runID, err)
	}
	return provider.WorkflowRun{
		ID:         run.GetID(),
		Status:     run.GetStatus(),
		Conclusion: run.GetConclusion(),
	}, nil
}

// ListCommitsSince returns commits reachable from head but not from base.
// When base is empty it lists the 100 most recent commits on head.
func (p *Provider) ListCommitsSince(ctx context.Context, owner, repo, base, head string) ([]provider.Commit, error) {
	if base == "" {
		commits, _, err := p.client.Repositories.ListCommits(ctx, owner, repo,
			&gh.CommitsListOptions{SHA: head, ListOptions: gh.ListOptions{PerPage: 100}})
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}
		return ghCommits(commits), nil
	}
	comp, _, err := p.client.Repositories.CompareCommits(ctx, owner, repo, base, head, &gh.ListOptions{PerPage: 250})
	if err != nil {
		return nil, fmt.Errorf("compare %s...%s: %w", base, head, err)
	}
	return ghCommits(comp.Commits), nil
}

// ListCommitFiles returns the paths touched by a single commit.
func (p *Provider) ListCommitFiles(ctx context.Context, owner, repo, sha string) ([]string, error) {
	commit, _, err := p.client.Repositories.GetCommit(ctx, owner, repo, sha, &gh.ListOptions{PerPage: 300})
	if err != nil {
		return nil, fmt.Errorf("get commit %s: %w", sha, err)
	}
	files := make([]string, 0, len(commit.Files))
	for _, f := range commit.Files {
		files = append(files, f.GetFilename())
	}
	return files, nil
}

// GenerateReleaseNotes uses GitHub's native release-notes generation API.
// targetCommitish is always supplied so this works even when tag does not
// exist yet on the remote (GitHub computes notes as if it were cut there).
func (p *Provider) GenerateReleaseNotes(ctx context.Context, owner, repo, tag, previousTag, targetCommitish string) (string, error) {
	opts := &gh.GenerateNotesOptions{TagName: tag}
	if previousTag != "" {
		opts.PreviousTagName = gh.String(previousTag)
	}
	if targetCommitish != "" {
		opts.TargetCommitish = gh.String(targetCommitish)
	}
	notes, _, err := p.client.Repositories.GenerateReleaseNotes(ctx, owner, repo, opts)
	if err != nil {
		return "", fmt.Errorf("generate release notes: %w", err)
	}
	return notes.Body, nil
}

func ghCommits(raw []*gh.RepositoryCommit) []provider.Commit {
	out := make([]provider.Commit, 0, len(raw))
	for _, c := range raw {
		msg := c.GetCommit().GetMessage()
		if i := strings.Index(msg, "\n"); i != -1 {
			msg = msg[:i]
		}
		out = append(out, provider.Commit{
			SHA:     c.GetSHA(),
			Message: msg,
			Author:  c.GetCommit().GetAuthor().GetName(),
		})
	}
	return out
}

// InstallWebhook creates or updates a webhook on the repository pointing at webhookURL.
// If a webhook for webhookURL already exists it is patched in-place; otherwise a new one is created.
func (p *Provider) InstallWebhook(ctx context.Context, owner, repo, webhookURL, secret string, events []string) error {
	hooks, _, err := p.client.Repositories.ListHooks(ctx, owner, repo, nil)
	if err != nil {
		return fmt.Errorf("list hooks: %w", err)
	}

	hookCfg := &gh.HookConfig{
		URL:         gh.String(webhookURL),
		ContentType: gh.String("json"),
		Secret:      gh.String(secret),
		InsecureSSL: gh.String("0"),
	}

	for _, h := range hooks {
		if h.GetConfig().GetURL() == webhookURL {
			_, _, err = p.client.Repositories.EditHook(ctx, owner, repo, h.GetID(), &gh.Hook{
				Config: hookCfg,
				Events: events,
				Active: gh.Bool(true),
			})
			return err
		}
	}

	_, _, err = p.client.Repositories.CreateHook(ctx, owner, repo, &gh.Hook{
		Name:   gh.String("web"),
		Config: hookCfg,
		Events: events,
		Active: gh.Bool(true),
	})
	return err
}

// CreateRelease creates a GitHub release and returns its HTML URL.
func (p *Provider) CreateRelease(ctx context.Context, owner, repo, tag, name, body string, draft, prerelease bool) (string, error) {
	rel, _, err := p.client.Repositories.CreateRelease(ctx, owner, repo, &gh.RepositoryRelease{
		TagName:    gh.String(tag),
		Name:       gh.String(name),
		Body:       gh.String(body),
		Draft:      gh.Bool(draft),
		Prerelease: gh.Bool(prerelease),
	})
	if err != nil {
		return "", fmt.Errorf("create release %s: %w", tag, err)
	}
	return rel.GetHTMLURL(), nil
}
