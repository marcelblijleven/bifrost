package steps

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
)

var dispatchPollInterval = 15 * time.Second

// SetDispatchPollInterval overrides the workflow run polling interval. Intended for tests.
func SetDispatchPollInterval(d time.Duration) { dispatchPollInterval = d }

// DispatchStep triggers a CI/CD workflow and optionally waits for completion.
type DispatchStep struct {
	workflow             string
	wait                 bool
	timeoutMinutes       int
	requireApproval      bool
	approvalMessage      string
	approvalTimeoutHours int
}

// NewDispatchStep constructs a DispatchStep from a config map.
//
// Required: "workflow" (string).
// Optional:
//   - "wait" (bool)                    — block until the workflow run completes
//   - "timeout_minutes" (int, 30)      — timeout for workflow completion wait
//   - "require_approval" (bool)        — gate dispatch behind a human approval
//   - "approval_message" (string)      — message shown in the approval UI
//   - "approval_timeout_hours" (int, 24) — timeout for the approval gate
func NewDispatchStep(cfg map[string]any) (pipeline.Step, error) {
	workflow, _ := cfg["workflow"].(string)
	if workflow == "" {
		return nil, fmt.Errorf("dispatch_workflow: 'workflow' config field is required")
	}

	wait, _ := cfg["wait"].(bool)
	requireApproval, _ := cfg["require_approval"].(bool)

	timeoutMinutes := 30
	if v, ok := cfg["timeout_minutes"]; ok {
		switch tv := v.(type) {
		case int:
			timeoutMinutes = tv
		case float64:
			timeoutMinutes = int(tv)
		}
	}

	approvalTimeoutHours := 24
	if v, ok := cfg["approval_timeout_hours"]; ok {
		switch tv := v.(type) {
		case int:
			approvalTimeoutHours = tv
		case float64:
			approvalTimeoutHours = int(tv)
		}
	}

	approvalMessage, _ := cfg["approval_message"].(string)

	return &DispatchStep{
		workflow:             workflow,
		wait:                 wait,
		timeoutMinutes:       timeoutMinutes,
		requireApproval:      requireApproval,
		approvalMessage:      approvalMessage,
		approvalTimeoutHours: approvalTimeoutHours,
	}, nil
}

func (s *DispatchStep) Name() string { return "dispatch_workflow:" + s.workflow }

func (s *DispatchStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	ref := sc.Tag
	if ref == "" {
		ref = sc.Event.Branch
	}

	var runID int64
	var runURL string
	if prior, ok := sc.PriorExternalRunIDs[sc.StepIndex]; ok && prior != 0 {
		// A previous execution of this run already dispatched the workflow and
		// was interrupted. Re-attach to that run instead of dispatching (and
		// re-approving) a second time.
		runID = prior
		slog.Info("dispatch_workflow: re-attaching to previously dispatched run",
			"workflow", s.workflow, "run_id", runID)
	} else {
		if s.requireApproval {
			msg := s.approvalMessage
			if msg == "" {
				msg = fmt.Sprintf("Approve dispatch of workflow %q?", s.workflow)
			}
			timeout := time.Duration(s.approvalTimeoutHours) * time.Hour
			if err := requestAndWaitForApproval(ctx, sc, s.Name(), msg, timeout); err != nil {
				return err
			}
		}

		var err error
		runID, runURL, err = sc.Provider.DispatchWorkflow(
			ctx,
			sc.Event.RepoOwner,
			sc.Event.RepoName,
			s.workflow,
			ref,
			map[string]string{},
		)
		if err != nil {
			return fmt.Errorf("dispatch %s: %w", s.workflow, err)
		}

		// Persist the run ID immediately (not just when the step completes):
		// if this process dies while waiting below, the resumed run re-attaches
		// to this workflow run instead of deploying a second time.
		if runID != 0 {
			if err := sc.Store.SetStepResultExternalRunID(ctx, sc.RunID, sc.StepIndex, runID); err != nil {
				slog.Warn("dispatch_workflow: failed to persist external run id",
					"workflow", s.workflow, "run_id", runID, "err", err)
			}
		}

		slog.Info("dispatch_workflow: dispatched", "workflow", s.workflow, "run_id", runID, "url", runURL)
	}

	sc.ExternalRunID = &runID

	out := map[string]any{"workflow": s.workflow, "ref": ref, "run_id": runID}
	if runURL != "" {
		out["url"] = runURL
	}

	if !s.wait {
		setOutput(sc, out)
		return nil
	}

	timeout := time.Duration(s.timeoutMinutes) * time.Minute
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(dispatchPollInterval)
	defer ticker.Stop()

	// Poll immediately, then on every tick: a re-attached workflow run may
	// already have completed while this run was being recovered.
	for {
		wr, err := sc.Provider.GetWorkflowRun(ctx, sc.Event.RepoOwner, sc.Event.RepoName, runID)
		switch {
		case err != nil:
			slog.Warn("dispatch_workflow: poll failed", "workflow", s.workflow, "err", err)
		case wr.Status != "completed":
			slog.Info("dispatch_workflow: waiting", "workflow", s.workflow, "status", wr.Status)
		default:
			slog.Info("dispatch_workflow: completed", "workflow", s.workflow, "conclusion", wr.Conclusion)
			out["conclusion"] = wr.Conclusion
			setOutput(sc, out)
			if wr.Conclusion == "success" || wr.Conclusion == "skipped" {
				return nil
			}
			return fmt.Errorf("workflow %s completed with conclusion: %s", s.workflow, wr.Conclusion)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("dispatch_workflow: timed out waiting for %s after %v", s.workflow, timeout)
		case <-ticker.C:
		}
	}
}
