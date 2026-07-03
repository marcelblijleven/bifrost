package api

import (
	"testing"

	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/store"
)

func TestShouldSkip_CommitPatterns(t *testing.T) {
	sc := store.SkipConditions{CommitPatterns: []string{"[skip ci]", "[docs]"}}

	cases := []struct {
		msg  string
		want bool
	}{
		{"fix: normal commit", false},
		{"docs: update readme [skip ci]", true},
		{"[docs] update API reference", true},
		{"feat: new feature", false},
	}
	for _, c := range cases {
		kind, _ := shouldSkip(sc, provider.PushEvent{CommitMsg: c.msg})
		if (kind != skipNone) != c.want {
			t.Errorf("CommitPatterns: msg=%q want skip=%v got kind=%v", c.msg, c.want, kind)
		}
		if c.want && kind != skipCommitPattern {
			t.Errorf("CommitPatterns: msg=%q want kind=skipCommitPattern got %v", c.msg, kind)
		}
	}
}

func TestShouldSkip_PathsIgnore(t *testing.T) {
	sc := store.SkipConditions{PathsIgnore: []string{"docs/**", "*.md"}}

	cases := []struct {
		files []string
		want  bool
	}{
		{[]string{"docs/guide.md", "docs/api.md"}, true}, // all match
		{[]string{"README.md"}, true},                    // *.md matches root
		{[]string{"src/main.go", "docs/guide.md"}, false}, // src/main.go not ignored
		{[]string{"src/main.go"}, false},                  // no ignore match
		{[]string{}, false},                               // no files → don't skip
	}
	for _, c := range cases {
		kind, _ := shouldSkip(sc, provider.PushEvent{ChangedFiles: c.files})
		if (kind != skipNone) != c.want {
			t.Errorf("PathsIgnore: files=%v want=%v got kind=%v", c.files, c.want, kind)
		}
		if c.want && kind != skipPathRouting {
			t.Errorf("PathsIgnore: files=%v want kind=skipPathRouting got %v", c.files, kind)
		}
	}
}

func TestShouldSkip_PathsInclude(t *testing.T) {
	sc := store.SkipConditions{PathsInclude: []string{"src/**", "internal/**"}}

	cases := []struct {
		files []string
		want  bool
	}{
		{[]string{"src/main.go"}, false},                  // matches → run
		{[]string{"internal/api/handler.go"}, false},      // matches → run
		{[]string{"docs/guide.md", "README.md"}, true},    // no match → skip
		{[]string{"src/main.go", "docs/guide.md"}, false}, // at least one match → run
		{[]string{}, false},                               // no files → don't skip
	}
	for _, c := range cases {
		kind, _ := shouldSkip(sc, provider.PushEvent{ChangedFiles: c.files})
		if (kind != skipNone) != c.want {
			t.Errorf("PathsInclude: files=%v want=%v got kind=%v", c.files, c.want, kind)
		}
		if c.want && kind != skipPathRouting {
			t.Errorf("PathsInclude: files=%v want kind=skipPathRouting got %v", c.files, kind)
		}
	}
}
