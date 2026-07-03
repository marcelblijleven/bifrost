package pathmatch

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**", "anything/goes", true},
		{"docs/**", "docs/guide.md", true},
		{"docs/**", "docs/sub/page.md", true},
		{"docs/**", "src/main.go", false},
		{"**/*.md", "README.md", true},
		{"**/*.md", "docs/guide.md", true},
		{"**/*.md", "src/main.go", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false}, // * doesn't cross /
		{"src/*.go", "src/main.go", true},
		{"src/*.go", "src/sub/main.go", false},
		// Tag-name globs (used by tag-triggered applications).
		{"v*", "v1.2.3", true},
		{"v*", "frontend-v1.2.3", false},
		{"frontend-v*", "frontend-v1.2.3", true},
	}
	for _, c := range cases {
		got := Match(c.pattern, c.path)
		if got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestAny(t *testing.T) {
	patterns := []string{"", "src/**", "*.md"}
	if !Any(patterns, "src/main.go") {
		t.Error("expected src/main.go to match src/**")
	}
	if Any(patterns, "docs/guide.txt") {
		t.Error("expected docs/guide.txt to match nothing")
	}
	if Any(nil, "anything") {
		t.Error("no patterns must match nothing")
	}
}
