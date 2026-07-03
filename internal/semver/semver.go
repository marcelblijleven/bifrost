package semver

import (
	"fmt"
	"sort"
	"strings"

	msemver "github.com/Masterminds/semver/v3"
)

// LatestVersionTag finds the highest semver tag from a slice of tag strings.
// Tags may be prefixed with "v". Returns "" if no valid semver tags are found.
func LatestVersionTag(tags []string) string {
	type pair struct {
		raw string
		ver *msemver.Version
	}
	var pairs []pair
	for _, t := range tags {
		v, err := msemver.NewVersion(t)
		if err == nil {
			pairs = append(pairs, pair{t, v})
		}
	}
	if len(pairs) == 0 {
		return ""
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[j].ver.LessThan(pairs[i].ver)
	})
	return pairs[0].raw
}

// NextVersionTag determines the next version tag by scanning commitMsgs (every
// commit since currentTag) for Conventional Commits markers and applying the
// highest-priority bump found across all of them:
//   - "BREAKING CHANGE" or "!:" in any message → major bump (minor bump if major == 0)
//   - "feat" prefix on any message → minor bump
//   - anything else → patch bump
//
// When vPrefix is false, the output never has a "v" prefix.
// When vPrefix is true, the output mirrors the prefix of currentTag so existing
// repo conventions are preserved automatically (default: "v0.1.0" for first tag).
func NextVersionTag(currentTag string, commitMsgs []string, vPrefix bool) (string, error) {
	if currentTag == "" {
		if vPrefix {
			return "v0.1.0", nil
		}
		return "0.1.0", nil
	}

	hasPrefix := strings.HasPrefix(currentTag, "v") || strings.HasPrefix(currentTag, "V")
	current, err := msemver.NewVersion(currentTag)
	if err != nil {
		return "", fmt.Errorf("parse current tag %q: %w", currentTag, err)
	}

	bump := "patch"
	for _, msg := range commitMsgs {
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(msg, "BREAKING CHANGE") || strings.Contains(msg, "!:"):
			bump = "major"
		case bump != "major" && strings.HasPrefix(lower, "feat"):
			bump = "minor"
		}
	}

	var next msemver.Version
	switch bump {
	case "major":
		if current.Major() == 0 {
			next = current.IncMinor()
		} else {
			next = current.IncMajor()
		}
	case "minor":
		next = current.IncMinor()
	default:
		next = current.IncPatch()
	}

	// vPrefix=false always strips "v"; vPrefix=true mirrors existing tag convention.
	return FormatTag(&next, vPrefix && hasPrefix), nil
}

// FormatTag formats v as a semver string, optionally with a "v" prefix.
func FormatTag(v *msemver.Version, vPrefix bool) string {
	if vPrefix {
		return "v" + v.String()
	}
	return v.String()
}
