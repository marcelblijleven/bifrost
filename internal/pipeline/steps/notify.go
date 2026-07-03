package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
)

// NotifyStep sends a JSON webhook POST when all prior steps have succeeded.
// A non-2xx response or network error is logged but does not fail the pipeline.
type NotifyStep struct {
	url     string
	headers map[string]string
}

func NewNotifyStep(cfg map[string]any) (pipeline.Step, error) {
	u, _ := cfg["url"].(string)
	if u == "" {
		return nil, fmt.Errorf("notify step: url is required")
	}
	s := &NotifyStep{url: u, headers: map[string]string{}}
	if h, ok := cfg["headers"].(map[string]any); ok {
		for k, v := range h {
			if sv, ok := v.(string); ok {
				s.headers[k] = sv
			}
		}
	}
	return s, nil
}

func (s *NotifyStep) Name() string { return "notify" }

func (s *NotifyStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	payload := map[string]any{
		"event":          "pipeline.success",
		"run_id":         sc.RunID.String(),
		"application_id": sc.ApplicationID.String(),
		"tag":            sc.Tag,
		"commit_sha":     sc.Event.CommitSHA,
		"branch":         sc.Event.Branch,
		"triggered_by":   sc.Event.AuthorName,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("notify: failed to build request", "url", s.url, "err", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("notify: request failed", "url", s.url, "err", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("notify: non-2xx response", "url", s.url, "status", resp.StatusCode)
	} else {
		slog.Info("notify: sent", "url", s.url, "status", resp.StatusCode)
	}
	setOutput(sc, map[string]any{"url": s.url, "status": resp.StatusCode})
	return nil
}
