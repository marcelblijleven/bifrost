package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/store"
)

// ErrLeaseLost is used as the context cancellation cause when this instance's
// ownership lease on the run is lost (expired or cancelled and reclaimed by
// another instance). The pipeline stops immediately and skips all further
// persistence, because another instance may already own the run's state.
var ErrLeaseLost = errors.New("run lease lost")

// abandoned reports whether the run's context was cancelled because ownership
// of the run was lost, as opposed to a user-initiated cancellation.
func abandoned(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrLeaseLost)
}

// Pipeline executes an ordered sequence of Steps, recording results to the store.
type Pipeline struct {
	Steps  []Step
	Notify func() // called after each step/run state change; may be nil
}

func New(steps []Step) *Pipeline {
	return &Pipeline{Steps: steps}
}

func (p *Pipeline) notify() {
	if p.Notify != nil {
		p.Notify()
	}
}

// Execute runs all steps in order.
// On step failure, remaining steps are marked skipped and the run is marked failed.
func (p *Pipeline) Execute(ctx context.Context, sc *StepContext, st store.Store, runID uuid.UUID) error {
	return p.ExecuteFrom(ctx, sc, st, runID, 0)
}

// ExecuteFrom resumes a run from fromStep. Steps before fromStep are restored
// (context-producing steps re-derive their output; side-effectful steps are
// skipped). Steps from fromStep onwards are executed normally.
//
// Persistence uses a context detached from ctx's cancellation so that a
// cancelled run still records its final state. The exception is a lost
// ownership lease (see ErrLeaseLost): then all persistence stops immediately.
func (p *Pipeline) ExecuteFrom(ctx context.Context, sc *StepContext, st store.Store, runID uuid.UUID, fromStep int) error {
	persistCtx := context.WithoutCancel(ctx)

	now := time.Now()
	if err := st.UpdatePipelineRun(persistCtx, &store.PipelineRun{
		ID:        runID,
		Status:    "running",
		StartedAt: &now,
	}); err != nil {
		return fmt.Errorf("mark run running: %w", err)
	}

	// Restore context from already-successful steps.
	for i, step := range p.Steps {
		if i >= fromStep {
			break
		}
		if r, ok := step.(Restorer); ok {
			if err := r.Restore(ctx, sc); err != nil {
				slog.Warn("context restore failed", "step", step.Name(), "err", err)
			}
		}
	}

	// Reset step results for steps from fromStep onwards.
	if err := st.DeleteStepResultsFrom(persistCtx, runID, fromStep); err != nil {
		return fmt.Errorf("reset step results: %w", err)
	}

	// Re-create pending results and execute from fromStep.
	results := make([]*store.StepResult, len(p.Steps)-fromStep)
	for i, step := range p.Steps[fromStep:] {
		r := &store.StepResult{
			ID:        uuid.New(),
			RunID:     runID,
			StepName:  step.Name(),
			StepIndex: fromStep + i,
			Status:    "pending",
		}
		results[i] = r
		if err := st.CreateStepResult(persistCtx, r); err != nil {
			slog.Error("create step result", "step", step.Name(), "err", err)
		}
	}

	var firstErr error
	for i, step := range p.Steps[fromStep:] {
		r := results[i]

		if firstErr != nil {
			r.Status = "skipped"
			if err := st.UpdateStepResult(persistCtx, r); err != nil {
				slog.Error("update step result skipped", "step", step.Name(), "err", err)
			}
			p.notify()
			continue
		}

		stepStart := time.Now()
		r.Status = "running"
		r.StartedAt = &stepStart
		if err := st.UpdateStepResult(persistCtx, r); err != nil {
			slog.Error("update step result running", "step", step.Name(), "err", err)
		}
		p.notify()

		sc.StepIndex = fromStep + i
		slog.Info("step starting", "step", step.Name(), "run_id", runID)
		err := step.Run(ctx, sc)
		stepEnd := time.Now()
		r.CompletedAt = &stepEnd

		if abandoned(ctx) {
			// Ownership of this run moved to another instance. Stop without
			// touching the store: the new owner manages the run's state now.
			slog.Warn("run lease lost; abandoning execution",
				"step", step.Name(), "run_id", runID)
			return ErrLeaseLost
		}

		if err != nil {
			switch {
			case errors.Is(err, ErrSuperseded):
				r.Status = "skipped"
			case errors.Is(err, context.Canceled):
				r.Status = "cancelled"
			default:
				r.Status = "failed"
				r.ErrorMessage = err.Error()
				slog.Error("step failed", "step", step.Name(), "run_id", runID,
					"duration", stepEnd.Sub(stepStart), "err", err)
			}
			firstErr = err
		} else {
			r.Status = "success"
			slog.Info("step succeeded", "step", step.Name(), "run_id", runID,
				"duration", stepEnd.Sub(stepStart))
		}
		r.ExternalRunID = sc.ExternalRunID
		r.Output = sc.Output
		sc.ExternalRunID = nil
		sc.Output = ""

		if err := st.UpdateStepResult(persistCtx, r); err != nil {
			slog.Error("update step result final", "step", step.Name(), "err", err)
		}
		p.notify()
	}

	completed := time.Now()
	finalStatus := "success"
	if firstErr != nil {
		switch {
		case errors.Is(firstErr, ErrSuperseded):
			finalStatus = "superseded"
		case errors.Is(firstErr, context.Canceled):
			finalStatus = "cancelled"
		default:
			finalStatus = "failed"
		}
	}
	if err := st.UpdatePipelineRun(persistCtx, &store.PipelineRun{
		ID:          runID,
		Status:      finalStatus,
		StartedAt:   &now,
		CompletedAt: &completed,
	}); err != nil {
		slog.Error("mark run complete", "run_id", runID, "err", err)
	}
	if finalStatus == "success" {
		if err := st.MarkRunReleased(persistCtx, runID); err != nil {
			slog.Warn("mark run released", "run_id", runID, "err", err)
		}
	}
	p.notify()

	return firstErr
}
