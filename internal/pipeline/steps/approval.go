package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/store"
)

var approvalPollInterval = 5 * time.Second

// SetApprovalPollInterval overrides the approval polling interval. Intended for tests.
func SetApprovalPollInterval(d time.Duration) { approvalPollInterval = d }

// ApprovalStep pauses the pipeline and waits for a human to approve or reject
// via the API before continuing.
type ApprovalStep struct {
	message string
	timeout time.Duration
}

// NewApprovalStep constructs an ApprovalStep from a config map.
// Optional: "message" (string), "timeout_hours" (int, default 24).
func NewApprovalStep(cfg map[string]any) (pipeline.Step, error) {
	message, _ := cfg["message"].(string)
	if message == "" {
		message = "Approval required to continue the pipeline"
	}
	timeoutHours := 24
	if v, ok := cfg["timeout_hours"]; ok {
		switch tv := v.(type) {
		case int:
			timeoutHours = tv
		case float64:
			timeoutHours = int(tv)
		}
	}
	return &ApprovalStep{
		message: message,
		timeout: time.Duration(timeoutHours) * time.Hour,
	}, nil
}

func (s *ApprovalStep) Name() string { return "approval" }

func (s *ApprovalStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	return requestAndWaitForApproval(ctx, sc, s.Name(), s.message, s.timeout)
}

// requestAndWaitForApproval creates an approval request and blocks until it is
// resolved, the context is cancelled, or the timeout elapses.
// It is shared by ApprovalStep and DispatchStep (when require_approval is set).
func requestAndWaitForApproval(ctx context.Context, sc *pipeline.StepContext, stepName, message string, timeout time.Duration) error {
	var req *store.ApprovalRequest

	// On recovery, an approval request for this run+step may already exist.
	// Inspect it so we don't create a duplicate and don't ignore a resolution
	// that occurred while the server was down.
	if existing, err := sc.Store.GetApprovalForStep(ctx, sc.RunID, sc.StepIndex); err == nil && existing != nil {
		switch existing.Status {
		case "approved":
			slog.Info("approval already granted before restart", "step", stepName, "run_id", sc.RunID)
			return nil
		case "rejected":
			return fmt.Errorf("rejected by %s", existing.ResolvedBy)
		case "superseded":
			return pipeline.ErrSuperseded
		case "pending":
			req = existing // reuse; do not create a duplicate
		}
	}

	if req == nil {
		req = &store.ApprovalRequest{
			ID:        uuid.New(),
			RunID:     sc.RunID,
			StepName:  stepName,
			StepIndex: sc.StepIndex,
			Message:   message,
		}
		if err := sc.Store.CreateApprovalRequest(ctx, req); err != nil {
			return fmt.Errorf("create approval request: %w", err)
		}
		if url := sc.Application.Notifications.OnApprovalURL; url != "" {
			go notifyApprovalRequested(url, sc.Application.Notifications.Headers, sc, req)
		}
	}

	slog.Info("waiting for approval",
		"step", stepName,
		"run_id", sc.RunID,
		"approval_id", req.ID,
		"message", message,
		"timeout", timeout,
	)

	ticker := time.NewTicker(approvalPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-deadline.C:
			return fmt.Errorf("approval timed out after %v", timeout)

		case <-sc.WakeApproval: // immediate wake when handler resolves the approval
		case <-ticker.C:
			current, err := sc.Store.GetApprovalRequest(ctx, req.ID)
			if err != nil {
				slog.Warn("approval poll failed", "step", stepName, "err", err)
				continue
			}
			switch current.Status {
			case "approved":
				slog.Info("approval granted", "step", stepName, "run_id", sc.RunID, "by", current.ResolvedBy)
				setOutput(sc, map[string]string{"status": "approved", "resolved_by": current.ResolvedBy})
				return nil
			case "rejected":
				return fmt.Errorf("rejected by %s", current.ResolvedBy)
			case "superseded":
				slog.Info("approval superseded", "step", stepName, "run_id", sc.RunID)
				return pipeline.ErrSuperseded
			}
		}
	}
}

// notifyApprovalRequested sends a JSON webhook POST when a pipeline pauses
// waiting on a new approval request, so someone finds out without having to
// watch the dashboard. Runs in its own goroutine; failures are logged only.
func notifyApprovalRequested(url string, headers map[string]string, sc *pipeline.StepContext, req *store.ApprovalRequest) {
	payload := map[string]any{
		"event":          "pipeline.approval_requested",
		"run_id":         sc.RunID.String(),
		"application_id": sc.ApplicationID.String(),
		"step_name":      req.StepName,
		"step_index":     req.StepIndex,
		"message":        req.Message,
		"branch":         sc.Event.Branch,
		"commit_sha":     sc.Event.CommitSHA,
		"triggered_by":   sc.Event.AuthorName,
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("approval notification: build request failed", "err", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Warn("approval notification: request failed", "url", url, "err", err)
		return
	}
	defer resp.Body.Close()
	slog.Info("approval notification: sent", "url", url, "status", resp.StatusCode)
}
