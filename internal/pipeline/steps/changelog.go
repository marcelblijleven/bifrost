package steps

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/marcelblijleven/bifrost/internal/pathmatch"
	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/provider"
)

// ChangelogStep generates a markdown changelog by listing commits since the previous tag.
type ChangelogStep struct{}

func (s *ChangelogStep) Name() string { return "changelog" }

// The changelog heading and release-notes generation both need sc.Tag.
func (s *ChangelogStep) Requires() []string { return []string{"semver"} }

func (s *ChangelogStep) Restore(ctx context.Context, sc *pipeline.StepContext) error {
	return s.Run(ctx, sc)
}

func (s *ChangelogStep) Run(ctx context.Context, sc *pipeline.StepContext) error {
	last, err := sc.Store.GetLastReleasedRun(ctx, sc.ApplicationID, sc.Event.Branch)
	if err != nil {
		slog.Warn("changelog: failed to look up last released run", "err", err)
	}

	var base string
	if last != nil {
		if last.Tag != "" {
			base = last.Tag
		} else {
			base = last.CommitSHA
		}
	}
	head := sc.Event.CommitSHA

	// When the application is scoped to paths (monorepo), the changelog must
	// only list commits touching those paths, which rules out the provider's
	// whole-repo release-notes generation.
	pathFilters := sc.Application.SkipConditions.PathsInclude

	// GitHub's generate-notes API requires previous_tag_name to be a real tag,
	// so only use it when we have one; a bare commit SHA baseline (or no prior
	// release) falls through to manual listing below, which accepts either.
	if gen, ok := sc.Provider.(provider.ReleaseNotesGenerator); ok && len(pathFilters) == 0 && sc.Tag != "" && last != nil && last.Tag != "" {
		notes, err := gen.GenerateReleaseNotes(ctx, sc.Event.RepoOwner, sc.Event.RepoName, sc.Tag, last.Tag, head)
		if err == nil {
			sc.Changelog = notes
			setOutput(sc, map[string]string{"tag": sc.Tag, "entry": sc.Changelog})
			return nil
		}
		slog.Warn("changelog: generate-notes API failed, falling back to manual changelog", "err", err)
	}

	commits, err := sc.Provider.ListCommitsSince(ctx, sc.Event.RepoOwner, sc.Event.RepoName, base, head)
	if err != nil {
		// Fall back to single-commit entry rather than failing the pipeline.
		sc.Changelog = fmt.Sprintf("## %s\n\n- %s\n", sc.Tag, sc.Event.CommitMsg)
		setOutput(sc, map[string]string{"tag": sc.Tag, "entry": sc.Changelog})
		return nil
	}

	commits = filterCommitsByPaths(ctx, sc, commits, pathFilters)

	sc.Changelog = formatChangelog(sc.Tag, commits)
	setOutput(sc, map[string]string{"tag": sc.Tag, "entry": sc.Changelog})
	return nil
}

// filterCommitsByPaths narrows commits to those touching at least one of the
// application's included paths. Filtering needs per-commit file lists from
// the provider; when the provider cannot supply them, or a lookup fails, the
// commit is kept rather than silently dropped.
func filterCommitsByPaths(ctx context.Context, sc *pipeline.StepContext, commits []provider.Commit, patterns []string) []provider.Commit {
	if len(patterns) == 0 || len(commits) == 0 {
		return commits
	}
	lister, ok := sc.Provider.(provider.CommitFilesLister)
	if !ok {
		slog.Warn("changelog: provider cannot list commit files; skipping path filtering",
			"provider", sc.Event.ProviderID)
		return commits
	}
	filtered := make([]provider.Commit, 0, len(commits))
	for _, c := range commits {
		files, err := lister.ListCommitFiles(ctx, sc.Event.RepoOwner, sc.Event.RepoName, c.SHA)
		if err != nil {
			slog.Warn("changelog: list commit files failed; keeping commit", "sha", c.SHA, "err", err)
			filtered = append(filtered, c)
			continue
		}
		for _, f := range files {
			if pathmatch.Any(patterns, f) {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered
}

var ccGroups = []struct {
	prefix string
	header string
}{
	{"feat", "Features"},
	{"fix", "Bug Fixes"},
	{"perf", "Performance"},
	{"refactor", "Refactoring"},
	{"docs", "Documentation"},
}

func formatChangelog(tag string, commits []provider.Commit) string {
	groups := make(map[string][]string)
	var other []string

	for _, c := range commits {
		lower := strings.ToLower(c.Message)
		matched := false
		for _, g := range ccGroups {
			if strings.HasPrefix(lower, g.prefix+":") || strings.HasPrefix(lower, g.prefix+"(") {
				groups[g.prefix] = append(groups[g.prefix], c.Message)
				matched = true
				break
			}
		}
		if !matched {
			skip := false
			for _, prefix := range []string{"chore:", "ci:", "build:", "style:", "test:"} {
				if strings.HasPrefix(lower, prefix) {
					skip = true
					break
				}
			}
			if !skip {
				other = append(other, c.Message)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(tag)
	sb.WriteString("\n")

	for _, g := range ccGroups {
		msgs := groups[g.prefix]
		if len(msgs) == 0 {
			continue
		}
		sb.WriteString("\n### ")
		sb.WriteString(g.header)
		sb.WriteString("\n")
		for _, m := range msgs {
			sb.WriteString("- ")
			sb.WriteString(m)
			sb.WriteString("\n")
		}
	}
	if len(other) > 0 {
		sb.WriteString("\n### Other\n")
		for _, m := range other {
			sb.WriteString("- ")
			sb.WriteString(m)
			sb.WriteString("\n")
		}
	}
	if sb.Len() == len("## "+tag+"\n") {
		// Nothing grouped — fall back to listing all commits
		sb.WriteString("\n")
		for _, c := range commits {
			sb.WriteString("- ")
			sb.WriteString(c.Message)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
