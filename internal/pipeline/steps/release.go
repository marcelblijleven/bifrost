package steps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/provider"
)

// CreateReleaseStep creates a VCS release using the tag and changelog from prior steps.
type CreateReleaseStep struct {
	draft      bool
	prerelease bool
}

func NewCreateReleaseStep(cfg map[string]any) (pipeline.Step, error) {
	s := &CreateReleaseStep{}
	if v, ok := cfg["draft"].(bool); ok {
		s.draft = v
	}
	if v, ok := cfg["prerelease"].(bool); ok {
		s.prerelease = v
	}
	return s, nil
}

func (s *CreateReleaseStep) Name() string { return "create_release" }

func (s *CreateReleaseStep) Requires() []string { return []string{"semver"} }

func (s *CreateReleaseStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	if sc.Tag == "" {
		return fmt.Errorf("create_release: tag is empty (semver step must run first)")
	}
	// A release for this tag may already exist because a previous execution of
	// this run created it and was interrupted before recording success.
	if url, err := sc.Provider.GetReleaseByTag(ctx, sc.Event.RepoOwner, sc.Event.RepoName, sc.Tag); err == nil {
		slog.Info("create_release: release already exists, treating as success",
			"tag", sc.Tag, "url", url)
		setOutput(sc, map[string]any{
			"tag":      sc.Tag,
			"url":      url,
			"draft":    s.draft,
			"existing": true,
		})
		return nil
	} else if !errors.Is(err, provider.ErrNotFound) {
		return fmt.Errorf("create_release: check existing release: %w", err)
	}
	url, err := sc.Provider.CreateRelease(ctx,
		sc.Event.RepoOwner, sc.Event.RepoName,
		sc.Tag, sc.Tag, sc.Changelog,
		s.draft, s.prerelease)
	if err != nil {
		return fmt.Errorf("create_release: %w", err)
	}
	setOutput(sc, map[string]any{
		"tag":   sc.Tag,
		"url":   url,
		"draft": s.draft,
	})
	return nil
}
