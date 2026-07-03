package steps

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
)

// TagStep creates an annotated git tag at the commit SHA determined by a prior SemverStep.
type TagStep struct{}

func (s *TagStep) Name() string { return "tag" }

func (s *TagStep) Requires() []string { return []string{"semver"} }

func (s *TagStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	if sc.Tag == "" {
		return fmt.Errorf("tag is empty: semver step must run before tag step")
	}
	msg := "Release " + sc.Tag
	if sc.Changelog != "" {
		msg = sc.Changelog
	}
	if err := sc.Provider.CreateTag(
		ctx,
		sc.Event.RepoOwner,
		sc.Event.RepoName,
		sc.Tag,
		sc.Event.CommitSHA,
		msg,
	); err != nil {
		// The tag may already exist because a previous execution of this run
		// created it and was interrupted before recording success. That is a
		// successful outcome; a tag pointing elsewhere is a genuine collision.
		existingSHA, lookupErr := sc.Provider.GetTagCommitSHA(ctx, sc.Event.RepoOwner, sc.Event.RepoName, sc.Tag)
		if lookupErr != nil {
			return err
		}
		if existingSHA != sc.Event.CommitSHA {
			return fmt.Errorf("tag %s already exists but points at %s, expected %s: %w",
				sc.Tag, existingSHA, sc.Event.CommitSHA, err)
		}
		slog.Info("tag: already exists at expected commit, treating as success",
			"tag", sc.Tag, "sha", sc.Event.CommitSHA)
	}
	setOutput(sc, map[string]string{"tag": sc.Tag, "sha": sc.Event.CommitSHA})
	return nil
}
