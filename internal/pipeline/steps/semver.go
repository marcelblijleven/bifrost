package steps

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
	bisemver "github.com/marcelblijleven/bifrost/internal/semver"
)

// SemverStep determines the next version tag from the repo's existing tags
// and every commit since the latest tag, using Conventional Commits rules.
type SemverStep struct {
	vPrefix bool // whether to prepend "v" to the first tag (default true)
}

// NewSemverStep constructs a SemverStep from a config map.
// Optional: "v_prefix" (bool, default true) — set false to omit the "v" prefix.
func NewSemverStep(cfg map[string]any) (pipeline.Step, error) {
	vPrefix := true
	if v, ok := cfg["v_prefix"].(bool); ok {
		vPrefix = v
	}
	return &SemverStep{vPrefix: vPrefix}, nil
}

func (s *SemverStep) Name() string { return "semver" }

// Restore reads the tag that was persisted when Run first executed so that
// retrying a later step uses the original computed version rather than
// re-deriving a new one from the (now-updated) git tag list.
func (s *SemverStep) Restore(ctx context.Context, sc *pipeline.StepContext) error {
	run, err := sc.Store.GetPipelineRun(ctx, sc.RunID)
	if err == nil && run.Tag != "" {
		sc.Tag = run.Tag
		return nil
	}
	// Fallback for runs that pre-date the tag column.
	return s.Run(ctx, sc)
}

func (s *SemverStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	tags, err := sc.Provider.ListTags(ctx, sc.Event.RepoOwner, sc.Event.RepoName)
	if err != nil {
		return fmt.Errorf("list tags: %w", err)
	}

	// The application's tag prefix namespaces its releases within the repo
	// (monorepo: several applications tag the same repository). Versions are
	// computed on the stripped names; the prefix is re-applied to the result.
	prefix := sc.Application.TagPrefix
	if prefix != "" {
		stripped := make([]string, 0, len(tags))
		for _, t := range tags {
			if strings.HasPrefix(t, prefix) {
				stripped = append(stripped, strings.TrimPrefix(t, prefix))
			}
		}
		tags = stripped
	}

	latest := bisemver.LatestVersionTag(tags)
	latestFull := latest
	if latest != "" {
		latestFull = prefix + latest
	}

	commitMsgs := []string{sc.Event.CommitMsg}
	commits, err := sc.Provider.ListCommitsSince(ctx, sc.Event.RepoOwner, sc.Event.RepoName, latestFull, sc.Event.CommitSHA)
	if err != nil {
		slog.Warn("semver: failed to list commits since last tag, falling back to triggering commit only", "err", err)
	} else if len(commits) > 0 {
		commitMsgs = make([]string, len(commits))
		for i, c := range commits {
			commitMsgs[i] = c.Message
		}
	}

	next, err := bisemver.NextVersionTag(latest, commitMsgs, s.vPrefix)
	if err != nil {
		return fmt.Errorf("determine next version: %w", err)
	}

	sc.Tag = prefix + next

	if err := sc.Store.UpdateRunTag(ctx, sc.RunID, sc.Tag); err != nil {
		slog.Warn("semver: failed to persist computed tag", "run_id", sc.RunID, "tag", sc.Tag, "err", err)
	}

	setOutput(sc, map[string]string{"tag": sc.Tag, "previous": latestFull})
	return nil
}
