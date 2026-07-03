package api

import (
	"strings"

	"github.com/marcelblijleven/bifrost/internal/pathmatch"
	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// skipKind classifies why a push was skipped: commit-message opt-outs are
// recorded as skipped runs, path-routing skips are only logged.
type skipKind int

const (
	skipNone skipKind = iota
	skipCommitPattern
	skipPathRouting
)

// shouldSkip returns (kind, reason) when the push event should not trigger a
// pipeline run according to the application's SkipConditions. Returns
// (skipNone, "") when the run should proceed.
func shouldSkip(sc store.SkipConditions, event provider.PushEvent) (skipKind, string) {
	// 1. Commit message patterns (no file list needed).
	for _, pattern := range sc.CommitPatterns {
		if pattern != "" && strings.Contains(event.CommitMsg, pattern) {
			return skipCommitPattern, "commit message matches skip pattern: " + pattern
		}
	}

	// Path-based rules only apply when we know which files changed.
	if len(event.ChangedFiles) == 0 {
		return skipNone, ""
	}

	// 2. paths_ignore: skip when ALL changed files are covered by an ignore pattern.
	if len(sc.PathsIgnore) > 0 {
		allIgnored := true
		for _, f := range event.ChangedFiles {
			if !pathmatch.Any(sc.PathsIgnore, f) {
				allIgnored = false
				break
			}
		}
		if allIgnored {
			return skipPathRouting, "all changed files match paths_ignore patterns"
		}
	}

	// 3. paths_include: skip when NO changed file matches an include pattern.
	if len(sc.PathsInclude) > 0 {
		anyIncluded := false
		for _, f := range event.ChangedFiles {
			if pathmatch.Any(sc.PathsInclude, f) {
				anyIncluded = true
				break
			}
		}
		if !anyIncluded {
			return skipPathRouting, "no changed files match paths_include patterns"
		}
	}

	return skipNone, ""
}
