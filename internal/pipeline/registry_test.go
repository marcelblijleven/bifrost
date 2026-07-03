package pipeline_test

import (
	"context"
	"strings"
	"testing"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/store"
)

type plainStep struct{ name string }

func (s *plainStep) Name() string                                         { return s.name }
func (s *plainStep) Run(_ context.Context, _ *pipeline.StepContext) error { return nil }

type dependentStep struct {
	plainStep
	requires []string
}

func (s *dependentStep) Requires() []string { return s.requires }

func newTestRegistry() *pipeline.Registry {
	r := pipeline.NewRegistry()
	r.Register("semver", func(map[string]any) (pipeline.Step, error) {
		return &plainStep{name: "semver"}, nil
	})
	r.Register("tag", func(map[string]any) (pipeline.Step, error) {
		return &dependentStep{plainStep: plainStep{name: "tag"}, requires: []string{"semver"}}, nil
	})
	r.Register("notify", func(map[string]any) (pipeline.Step, error) {
		return &plainStep{name: "notify"}, nil
	})
	return r
}

func TestRegistryBuild_OrderingSatisfied(t *testing.T) {
	r := newTestRegistry()
	steps, err := r.Build([]store.StepConfig{{Type: "semver"}, {Type: "tag"}, {Type: "notify"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(steps))
	}
}

func TestRegistryBuild_OrderingViolated(t *testing.T) {
	r := newTestRegistry()
	_, err := r.Build([]store.StepConfig{{Type: "tag"}, {Type: "semver"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `requires step "semver"`) {
		t.Errorf("error = %q, want it to mention the missing requirement", err.Error())
	}
}

func TestRegistryBuild_MissingRequirementEntirely(t *testing.T) {
	r := newTestRegistry()
	_, err := r.Build([]store.StepConfig{{Type: "tag"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRegistryBuild_PresatisfiedRequirement(t *testing.T) {
	// Tag-triggered applications seed the release tag from the pushed tag, so
	// "semver" counts as satisfied without appearing in the pipeline.
	r := newTestRegistry()
	steps, err := r.Build([]store.StepConfig{{Type: "tag"}, {Type: "notify"}}, "semver")
	if err != nil {
		t.Fatalf("Build with presatisfied semver: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
}
