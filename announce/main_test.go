package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBackFediverseContext(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		url      string
		wantLine string
	}{
		{
			name: "yaml empty quoted value",
			input: `---
title: "Hello"
fediverse_context: ""
tags:
  - foo
---
body`,
			url:      "https://m.example/@u/123",
			wantLine: `fediverse_context: "https://m.example/@u/123"`,
		},
		{
			name: "yaml empty bare value",
			input: `---
title: "Hello"
fediverse_context:
---
body`,
			url:      "https://m.example/@u/123",
			wantLine: `fediverse_context: "https://m.example/@u/123"`,
		},
		{
			name: "toml empty value",
			input: `+++
title = "Hello"
fediverse_context = ""
+++
body`,
			url:      "https://m.example/@u/456",
			wantLine: `fediverse_context = "https://m.example/@u/456"`,
		},
		{
			name: "url with backslash and quote gets escaped",
			input: `---
fediverse_context: ""
---
`,
			url:      `https://m.example/weird"path\here`,
			wantLine: `fediverse_context: "https://m.example/weird\"path\\here"`,
		},
		{
			name: "url containing $1 is not interpreted as backref",
			input: `---
fediverse_context: ""
---
`,
			url:      `https://m.example/$1/$2`,
			wantLine: `fediverse_context: "https://m.example/$1/$2"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "post.md")
			if err := os.WriteFile(path, []byte(tc.input), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := writeBackFediverseContext(path, []byte(tc.input), tc.url); err != nil {
				t.Fatalf("writeBack: %v", err)
			}
			out, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), tc.wantLine) {
				t.Errorf("expected line %q in output:\n%s", tc.wantLine, string(out))
			}
			// Front-matter structure must be otherwise unchanged: other lines preserved.
			for _, line := range strings.Split(tc.input, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "fediverse_context") {
					continue
				}
				if line == "" {
					continue
				}
				if !strings.Contains(string(out), line) {
					t.Errorf("line %q lost from output:\n%s", line, string(out))
				}
			}
		})
	}
}

func TestWriteBackFediverseContext_KeyAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "post.md")
	raw := []byte("---\ntitle: \"Hi\"\n---\nbody")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeBackFediverseContext(path, raw, "https://m.example/@u/1")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestScanCandidates(t *testing.T) {
	contentDir := filepath.Join(t.TempDir(), "content")
	if err := os.MkdirAll(filepath.Join(contentDir, "blog"), 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"candidate-yaml.md": `---
title: "Yes"
fediverse_context: ""
---
body`,
		"candidate-toml.md": `+++
title = "Yes"
fediverse_context = ""
+++
body`,
		"candidate-whitespace-only.md": `---
fediverse_context: "   "
---`,
		"blog/candidate-nested.md": `---
fediverse_context: ""
---`,
		"already-announced.md": `---
fediverse_context: "https://m.example/@u/1"
---`,
		"no-key.md": `---
title: "Skip me"
---`,
		"not-markdown.txt": `---
fediverse_context: ""
---`,
		"missing-front-matter.md": `# just markdown, no front matter`,
	}
	for name, body := range files {
		path := filepath.Join(contentDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := scanCandidates([]string{contentDir})
	if err != nil {
		t.Fatalf("scanCandidates: %v", err)
	}

	wantPaths := []string{
		filepath.Join(contentDir, "candidate-yaml.md"),
		filepath.Join(contentDir, "candidate-toml.md"),
		filepath.Join(contentDir, "candidate-whitespace-only.md"),
		filepath.Join(contentDir, "blog/candidate-nested.md"),
	}
	if len(got) != len(wantPaths) {
		t.Fatalf("len=%d want=%d (got=%v)", len(got), len(wantPaths), got)
	}
	for _, p := range wantPaths {
		if !got[p] {
			t.Errorf("missing candidate %q in result", p)
		}
	}
}

func TestScanCandidates_MultipleRoots(t *testing.T) {
	// Simulate two content roots (e.g. project + theme overlay).
	root1 := filepath.Join(t.TempDir(), "content")
	root2 := filepath.Join(t.TempDir(), "content")
	for _, r := range []string{root1, root2} {
		if err := os.MkdirAll(r, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root1, "a.md"), []byte("---\nfediverse_context: \"\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root2, "b.md"), []byte("---\nfediverse_context: \"\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanCandidates([]string{root1, root2})
	if err != nil {
		t.Fatalf("scanCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates from two roots, got %d (%v)", len(got), got)
	}
	if !got[filepath.Join(root1, "a.md")] || !got[filepath.Join(root2, "b.md")] {
		t.Errorf("missing expected paths in %v", got)
	}
}

func TestScanCandidates_MissingRootIsError(t *testing.T) {
	_, err := scanCandidates([]string{filepath.Join(t.TempDir(), "does-not-exist")})
	if err == nil {
		t.Fatal("expected error when root is missing")
	}
}

func TestExtractHashtags(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{name: "nil", in: nil, want: nil},
		{name: "string not list", in: "blog", want: nil},
		{
			name: "yaml-style []any",
			in:   []any{"blog", "france", "lowtech"},
			want: []string{"#blog", "#france", "#lowtech"},
		},
		{
			name: "[]string",
			in:   []string{"blog", "france"},
			want: []string{"#blog", "#france"},
		},
		{
			name: "strips leading hash and trims",
			in:   []any{"  #blog ", "#france", " lowtech"},
			want: []string{"#blog", "#france", "#lowtech"},
		},
		{
			name: "drops empty and non-string entries",
			in:   []any{"blog", "", "  ", 42, nil, "france"},
			want: []string{"#blog", "#france"},
		},
		{
			name: "empty list",
			in:   []any{},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractHashtags(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want=%d (got=%v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d]=%q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestMergeHashtags(t *testing.T) {
	cases := []struct {
		name     string
		defaults []string
		page     []string
		want     []string
	}{
		{name: "both nil", defaults: nil, page: nil, want: []string{}},
		{
			name:     "defaults only",
			defaults: []string{"#blog", "#france"},
			page:     nil,
			want:     []string{"#blog", "#france"},
		},
		{
			name:     "page only",
			defaults: nil,
			page:     []string{"#lowtech"},
			want:     []string{"#lowtech"},
		},
		{
			name:     "defaults lead, page appended",
			defaults: []string{"#blog"},
			page:     []string{"#lowtech", "#france"},
			want:     []string{"#blog", "#lowtech", "#france"},
		},
		{
			name:     "case-insensitive dedup keeps default position",
			defaults: []string{"#Blog", "#france"},
			page:     []string{"#blog", "#lowtech"},
			want:     []string{"#Blog", "#france", "#lowtech"},
		},
		{
			name:     "duplicate within page dropped",
			defaults: nil,
			page:     []string{"#a", "#A", "#b"},
			want:     []string{"#a", "#b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeHashtags(tc.defaults, tc.page)
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want=%d (got=%v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d]=%q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNormaliseInstance(t *testing.T) {
	cases := map[string]string{
		"mastodon.social":          "mastodon.social",
		"https://mastodon.social":  "mastodon.social",
		"http://mastodon.social/":  "mastodon.social",
		"https://mastodon.social/": "mastodon.social",
		"":                         "",
	}
	for in, want := range cases {
		if got := normaliseInstance(in); got != want {
			t.Errorf("normaliseInstance(%q) = %q, want %q", in, got, want)
		}
	}
}
