package semver_test

import (
	"testing"

	"github.com/marcelblijleven/bifrost/internal/semver"
)

func TestLatestVersionTag(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"empty", nil, ""},
		{"no valid semver", []string{"not-a-version", "also-not"}, ""},
		{"single v-prefix", []string{"v1.2.3"}, "v1.2.3"},
		{"picks highest", []string{"v1.0.0", "v2.0.0", "v1.9.9"}, "v2.0.0"},
		{"mixed prefix and bare", []string{"1.0.0", "v2.0.0", "v1.5.0"}, "v2.0.0"},
		// v2.0.0-alpha.1 > v1.0.0 because major 2 > 1; pre-release only matters within same major.minor.patch
		{"pre-release on higher major wins", []string{"v1.0.0", "v2.0.0-alpha.1"}, "v2.0.0-alpha.1"},
		// v1.0.0-alpha.1 < v1.0.0 — pre-release is lower than stable at same version
		{"pre-release lower than stable same version", []string{"v1.0.0", "v1.0.0-alpha.1"}, "v1.0.0"},
		{"ignores non-semver in mixed list", []string{"latest", "v0.1.0", "v0.2.0"}, "v0.2.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := semver.LatestVersionTag(tc.tags)
			if got != tc.want {
				t.Errorf("LatestVersionTag(%v) = %q, want %q", tc.tags, got, tc.want)
			}
		})
	}
}

func TestNextVersionTag(t *testing.T) {
	cases := []struct {
		name       string
		currentTag string
		commitMsg  string
		vPrefix    bool
		want       string
		wantErr    bool
	}{
		// v-prefix behaviour (default)
		{"no current tag returns v0.1.0", "", "fix: anything", true, "v0.1.0", false},
		{"patch bump on fix", "v1.2.3", "fix: something", true, "v1.2.4", false},
		{"patch bump on chore", "v1.2.3", "chore: update deps", true, "v1.2.4", false},
		{"minor bump on feat", "v1.2.3", "feat: new feature", true, "v1.3.0", false},
		{"feat case insensitive", "v1.2.3", "Feat: new feature", true, "v1.3.0", false},
		{"major bump on BREAKING CHANGE", "v1.2.3", "fix!: BREAKING CHANGE in something", true, "v2.0.0", false},
		{"major bump on bang colon", "v1.2.3", "feat!: drop old API", true, "v2.0.0", false},
		{"no major bump when major is 0", "v0.2.3", "BREAKING CHANGE: drop API", true, "v0.3.0", false},
		{"invalid current tag returns error", "not-semver", "fix: x", true, "", true},
		// bare tag mirrors existing bare prefix
		{"bare current tag stays bare", "1.0.0", "fix: x", true, "1.0.1", false},
		{"bare minor bump stays bare", "1.2.3", "feat: new thing", true, "1.3.0", false},
		// no-prefix first tag
		{"no current tag no prefix", "", "fix: anything", false, "0.1.0", false},
		{"bare first tag with feat", "", "feat: new thing", false, "0.1.0", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := semver.NextVersionTag(tc.currentTag, []string{tc.commitMsg}, tc.vPrefix)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NextVersionTag(%q, %q, %v) err = %v, wantErr = %v", tc.currentTag, tc.commitMsg, tc.vPrefix, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("NextVersionTag(%q, %q, %v) = %q, want %q", tc.currentTag, tc.commitMsg, tc.vPrefix, got, tc.want)
			}
		})
	}
}

func TestNextVersionTag_ScansAllCommits(t *testing.T) {
	cases := []struct {
		name       string
		currentTag string
		commitMsgs []string
		want       string
	}{
		{
			"feat buried under a later fix still bumps minor",
			"v1.2.3",
			[]string{"feat: new thing", "fix: typo", "chore: cleanup"},
			"v1.3.0",
		},
		{
			"breaking change buried under later commits still bumps major",
			"v1.2.3",
			[]string{"feat!: drop old API", "fix: typo", "chore: cleanup"},
			"v2.0.0",
		},
		{
			"all patch-level commits bump patch",
			"v1.2.3",
			[]string{"fix: a", "chore: b", "docs: c"},
			"v1.2.4",
		},
		{
			"no commits falls back to patch bump",
			"v1.2.3",
			nil,
			"v1.2.4",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := semver.NextVersionTag(tc.currentTag, tc.commitMsgs, true)
			if err != nil {
				t.Fatalf("NextVersionTag(%q, %v, true) unexpected err = %v", tc.currentTag, tc.commitMsgs, err)
			}
			if got != tc.want {
				t.Errorf("NextVersionTag(%q, %v, true) = %q, want %q", tc.currentTag, tc.commitMsgs, got, tc.want)
			}
		})
	}
}
