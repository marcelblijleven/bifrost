package pipeline

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// ErrSuperseded is returned by an approval step when a newer run's approval was
// granted, making this run obsolete. The pipeline marks the run as "superseded".
var ErrSuperseded = errors.New("superseded by a newer run")

// Step is a single unit of work in a release pipeline.
type Step interface {
	Name() string
	Run(ctx context.Context, sc *StepContext) error
}

// Restorer may be implemented by steps that produce shared context (e.g. sc.Tag).
// When a pipeline resumes from a later step, Restore is called on earlier steps
// to cheaply re-derive their context without re-executing side effects.
type Restorer interface {
	Restore(ctx context.Context, sc *StepContext) error
}

// Requires may be implemented by steps that depend on another step type having
// already run earlier in the same pipeline (e.g. TagStep needs SemverStep to
// have set sc.Tag). Registry.Build validates these constraints against the
// configured step order before instantiating the pipeline.
type Requires interface {
	Requires() []string
}

// StepContext carries the push event and mutable release state through the pipeline.
// Store, RunID, and StepIndex are set by Pipeline.Execute before each step runs.
type StepContext struct {
	Event         provider.PushEvent
	Provider      provider.Provider
	Application   store.Application
	Store         store.Store
	RunID         uuid.UUID
	ApplicationID uuid.UUID
	StepIndex     int
	// WakeApproval is signalled by the API handler when an approval is resolved,
	// allowing the approval step to react immediately instead of waiting for its
	// next poll tick. A nil channel is safe: the select case blocks forever.
	WakeApproval <-chan struct{}
	// PriorExternalRunIDs maps step index → external workflow run ID persisted
	// by an earlier, interrupted execution of this run. Dispatch steps use it to
	// re-attach to an already-dispatched workflow instead of dispatching again.
	// A nil map is safe.
	PriorExternalRunIDs map[int]int64
	// Mutable fields set by steps:
	Tag           string
	Changelog     string
	ExternalRunID *int64 // set by dispatch steps; pipeline copies to StepResult then resets
	Output        string // JSON summary set by steps; pipeline copies to StepResult then resets
}
