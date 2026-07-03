// Package pathmatch implements the glob syntax bifrost uses for path filters:
// skip conditions on incoming pushes and per-application changelog scoping.
package pathmatch

import (
	"path/filepath"
	"strings"
)

// Any reports whether path matches at least one of the patterns.
// Empty patterns are ignored.
func Any(patterns []string, path string) bool {
	for _, p := range patterns {
		if p != "" && Match(p, path) {
			return true
		}
	}
	return false
}

// Match matches a single glob pattern against a file path.
// Supported syntax:
//   - Standard filepath.Match patterns (*, ?, [range])
//   - "**" matches across path separators
//   - "dir/**" matches anything under dir/
//   - "**/*.ext" matches any file with that extension at any depth
func Match(pattern, path string) bool {
	if pattern == "**" {
		return true
	}
	// "**/" prefix: match base name or full path.
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		if m, _ := filepath.Match(suffix, filepath.Base(path)); m {
			return true
		}
		if m, _ := filepath.Match(suffix, path); m {
			return true
		}
		// Also check each subpath segment (e.g. "**/*.md" against "a/b/c.md").
		parts := strings.Split(path, "/")
		for i := range parts {
			sub := strings.Join(parts[i:], "/")
			if m, _ := filepath.Match(suffix, sub); m {
				return true
			}
		}
		return false
	}
	// "dir/**" suffix: prefix match.
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(path, prefix+"/")
	}
	m, _ := filepath.Match(pattern, path)
	return m
}
