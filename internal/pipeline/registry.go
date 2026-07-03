package pipeline

import (
	"fmt"

	"github.com/marcelblijleven/bifrost/internal/store"
)

// StepFactory creates a Step from its configuration map.
type StepFactory func(cfg map[string]any) (Step, error)

// Registry maps step type names to their factories.
type Registry struct {
	factories map[string]StepFactory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]StepFactory)}
}

// Register associates a step type name with its factory.
func (r *Registry) Register(stepType string, f StepFactory) {
	r.factories[stepType] = f
}

// Build instantiates steps from a project's pipeline config, validating that
// any step-ordering requirements (see Requires) are satisfied by the configured
// order. satisfied lists step types whose requirements are met outside the
// pipeline: tag-triggered applications seed the release tag from the pushed
// tag, satisfying "semver" without running the step.
func (r *Registry) Build(configs []store.StepConfig, satisfied ...string) ([]Step, error) {
	steps := make([]Step, 0, len(configs))
	seen := make(map[string]bool, len(configs)+len(satisfied))
	for _, s := range satisfied {
		seen[s] = true
	}
	for i, cfg := range configs {
		f, ok := r.factories[cfg.Type]
		if !ok {
			return nil, fmt.Errorf("unknown step type %q", cfg.Type)
		}
		s, err := f(cfg.Config)
		if err != nil {
			return nil, fmt.Errorf("build step %q: %w", cfg.Type, err)
		}
		if req, ok := s.(Requires); ok {
			for _, need := range req.Requires() {
				if !seen[need] {
					return nil, fmt.Errorf("step %d %q: requires step %q earlier in the pipeline", i, cfg.Type, need)
				}
			}
		}
		seen[cfg.Type] = true
		steps = append(steps, s)
	}
	return steps, nil
}
