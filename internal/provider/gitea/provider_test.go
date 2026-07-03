package gitea

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marcelblijleven/bifrost/internal/provider"
)

const secret = "test-secret"

func sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func makeRequest(t *testing.T, eventType string, payload any) *http.Request {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Event", eventType)
	req.Header.Set("X-Gitea-Signature", sign(body))
	return req
}

func TestParseWebhook_Push(t *testing.T) {
	payload := giteaPushPayload{
		Ref:   "refs/heads/main",
		After: "abc123def456",
		HeadCommit: &struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		}{
			ID:      "abc123def456",
			Message: "feat: add new feature",
			Author: struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			}{Name: "Alice", Email: "alice@example.com"},
		},
		Commits: []struct {
			ID       string   `json:"id"`
			Message  string   `json:"message"`
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
			Removed  []string `json:"removed"`
			Author   struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		}{
			{Added: []string{"src/main.go"}, Modified: []string{"go.mod"}},
		},
	}
	payload.Repository.Name = "my-repo"
	payload.Repository.Owner.Login = "my-org"

	p := New("gitea", "https://gitea.example.com", "token")
	ev, err := p.ParseWebhook(makeRequest(t, "push", payload), secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Branch != "main" {
		t.Errorf("branch = %q, want %q", ev.Branch, "main")
	}
	if ev.CommitSHA != "abc123def456" {
		t.Errorf("sha = %q, want %q", ev.CommitSHA, "abc123def456")
	}
	if ev.CommitMsg != "feat: add new feature" {
		t.Errorf("msg = %q, want %q", ev.CommitMsg, "feat: add new feature")
	}
	if ev.AuthorName != "Alice" {
		t.Errorf("author = %q, want %q", ev.AuthorName, "Alice")
	}
	if ev.RepoOwner != "my-org" || ev.RepoName != "my-repo" {
		t.Errorf("repo = %s/%s, want my-org/my-repo", ev.RepoOwner, ev.RepoName)
	}
	if len(ev.ChangedFiles) != 2 {
		t.Errorf("changed files = %d, want 2", len(ev.ChangedFiles))
	}
	if ev.ProviderID != "gitea" {
		t.Errorf("provider id = %q, want gitea", ev.ProviderID)
	}
}

func TestParseWebhook_WrongSignature(t *testing.T) {
	payload := map[string]any{"ref": "refs/heads/main", "after": "abc", "repository": map[string]any{"name": "r", "owner": map[string]any{"login": "o"}}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Gitea-Event", "push")
	req.Header.Set("X-Gitea-Signature", "badsignature")

	p := New("gitea", "https://gitea.example.com", "token")
	_, err := p.ParseWebhook(req, secret)
	if err == nil {
		t.Error("expected signature error, got nil")
	}
}

func TestParseWebhook_NotPush(t *testing.T) {
	payload := map[string]any{"action": "opened"}
	p := New("gitea", "https://gitea.example.com", "token")
	_, err := p.ParseWebhook(makeRequest(t, "issues", payload), secret)
	if err != provider.ErrNotPushEvent {
		t.Errorf("expected ErrNotPushEvent, got %v", err)
	}
}

func TestParseWebhook_TagPush(t *testing.T) {
	// A tag push (ref = refs/tags/v1.0.0) yields an event with TagName set
	// and no branch.
	payload := map[string]any{
		"ref": "refs/tags/v1.0.0", "after": "abc",
		"repository": map[string]any{"name": "r", "owner": map[string]any{"login": "o"}},
	}
	p := New("gitea", "https://gitea.example.com", "token")
	ev, err := p.ParseWebhook(makeRequest(t, "push", payload), secret)
	if err != nil {
		t.Fatalf("expected tag push to parse, got %v", err)
	}
	if ev.TagName != "v1.0.0" || ev.Branch != "" {
		t.Errorf("tag=%q branch=%q, want tag v1.0.0 and empty branch", ev.TagName, ev.Branch)
	}
	if ev.CommitSHA != "abc" {
		t.Errorf("sha = %q, want abc", ev.CommitSHA)
	}
}

func TestParseWebhook_CreateTagEvent(t *testing.T) {
	// Gitea/Forgejo deliver tag creation as a "create" event with ref_type tag.
	payload := map[string]any{
		"ref": "v2.0.0", "ref_type": "tag", "sha": "def",
		"sender":     map[string]any{"login": "alice"},
		"repository": map[string]any{"name": "r", "owner": map[string]any{"login": "o"}},
	}
	p := New("gitea", "https://gitea.example.com", "token")
	ev, err := p.ParseWebhook(makeRequest(t, "create", payload), secret)
	if err != nil {
		t.Fatalf("expected create tag event to parse, got %v", err)
	}
	if ev.TagName != "v2.0.0" || ev.CommitSHA != "def" || ev.AuthorName != "alice" {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestParseWebhook_CreateBranchEvent_Ignored(t *testing.T) {
	payload := map[string]any{
		"ref": "feature", "ref_type": "branch", "sha": "def",
		"repository": map[string]any{"name": "r", "owner": map[string]any{"login": "o"}},
	}
	p := New("gitea", "https://gitea.example.com", "token")
	_, err := p.ParseWebhook(makeRequest(t, "create", payload), secret)
	if err != provider.ErrNotPushEvent {
		t.Errorf("expected ErrNotPushEvent for branch create, got %v", err)
	}
}

func TestForgejoProviderID(t *testing.T) {
	p := New("forgejo", "https://codeberg.org", "token")
	if p.ID() != "forgejo" {
		t.Errorf("id = %q, want forgejo", p.ID())
	}
}
