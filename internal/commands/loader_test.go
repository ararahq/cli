package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupCommandFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoader_DiscoversUserCommands(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "hello.md",
		"---\ndescription: Says hi\nargHint: \"<name>\"\n---\necho hi $1\n")

	loader := NewLoader(t.TempDir())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 command, got %d", len(discovered))
	}
	got := discovered[0]
	if got.Name != "hello" {
		t.Errorf("name: want hello, got %q", got.Name)
	}
	if got.Source != SourceUser {
		t.Errorf("source: want user, got %q", got.Source)
	}
	if got.Description != "Says hi" {
		t.Errorf("description: want %q, got %q", "Says hi", got.Description)
	}
	if got.ArgHint != "<name>" {
		t.Errorf("argHint: want <name>, got %q", got.ArgHint)
	}
	if !strings.Contains(got.Body, "echo hi $1") {
		t.Errorf("body should contain echo line, got %q", got.Body)
	}
}

func TestLoader_ProjectOverridesUser(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "deploy.md",
		"---\ndescription: User-level deploy\n---\necho user\n")

	projectDir := t.TempDir()
	setupCommandFile(t, filepath.Join(projectDir, ".arara", "commands"), "deploy.md",
		"---\ndescription: Project deploy\n---\necho project\n")

	loader := NewLoader(projectDir)
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 command after dedupe, got %d", len(discovered))
	}
	if discovered[0].Description != "Project deploy" {
		t.Errorf("project should win, got %+v", discovered[0])
	}
	if discovered[0].Source != SourceProject {
		t.Errorf("source: want project, got %q", discovered[0].Source)
	}
}

func TestLoader_ProjectWalksUpAncestors(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	root := t.TempDir()
	setupCommandFile(t, filepath.Join(root, ".arara", "commands"), "promo.md",
		"---\ndescription: Run promo\n---\necho running\n")

	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(deep)
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 || discovered[0].Name != "promo" {
		t.Errorf("expected to find promo via ancestor walk, got %+v", discovered)
	}
}

func TestLoader_NoCommands(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	loader := NewLoader(t.TempDir())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected zero commands, got %d", len(discovered))
	}
}

func TestLoader_NoFrontmatter_UsesFirstBodyLine(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "bare.md",
		"# this is a comment header\n\necho the first non-comment line\n")

	loader := NewLoader(t.TempDir())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 command, got %d", len(discovered))
	}
	if discovered[0].Description != "echo the first non-comment line" {
		t.Errorf("description should fallback to first body line, got %q", discovered[0].Description)
	}
}

func TestLoader_BadFrontmatterIsSkipped(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "good.md",
		"---\ndescription: ok\n---\necho ok\n")
	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "bad.md",
		"---\nthis is not closed\n\necho missing terminator\n")

	loader := NewLoader(t.TempDir())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 || discovered[0].Name != "good" {
		t.Errorf("expected only 'good' to load, got %+v", discovered)
	}
}

func TestLoader_EmptyBodyRejected(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "empty.md",
		"---\ndescription: nothing\n---\n   \n")

	loader := NewLoader(t.TempDir())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 0 {
		t.Errorf("empty body should be skipped, got %+v", discovered)
	}
}

func TestLoader_IgnoresNonMarkdownFiles(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "README.txt",
		"this is not markdown")
	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "real.md",
		"---\ndescription: real\n---\necho real\n")

	loader := NewLoader(t.TempDir())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 {
		t.Errorf("expected only .md files, got %d", len(discovered))
	}
}

func TestLoader_Lookup(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	setupCommandFile(t, filepath.Join(tempHome, ".arara", "commands"), "foo.md",
		"---\ndescription: foo\n---\necho foo\n")

	loader := NewLoader(t.TempDir())

	got, err := loader.Lookup("foo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "foo" {
		t.Errorf("want foo, got %q", got.Name)
	}

	if _, err := loader.Lookup("missing"); err == nil {
		t.Error("expected error on missing command")
	}
}

func TestSplitFrontmatter(t *testing.T) {
	cases := []struct {
		name            string
		input           string
		wantFrontmatter string
		wantBody        string
		wantErr         bool
	}{
		{
			name:            "with frontmatter",
			input:           "---\nx: 1\n---\nbody here\n",
			wantFrontmatter: "x: 1",
			wantBody:        "body here\n",
		},
		{
			name:     "no frontmatter",
			input:    "just body\n",
			wantBody: "just body\n",
		},
		{
			name:    "unterminated",
			input:   "---\nx: 1\nbody no terminator",
			wantErr: true,
		},
		{
			name:            "leading newlines",
			input:           "\n\n---\nx: 1\n---\nbody\n",
			wantFrontmatter: "x: 1",
			wantBody:        "body\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := splitFrontmatter(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fm != tc.wantFrontmatter {
				t.Errorf("frontmatter: want %q, got %q", tc.wantFrontmatter, fm)
			}
			if body != tc.wantBody {
				t.Errorf("body: want %q, got %q", tc.wantBody, body)
			}
		})
	}
}

func TestCommandNameFromPath(t *testing.T) {
	if got := commandNameFromPath("/x/y/foo.md"); got != "foo" {
		t.Errorf("want foo, got %q", got)
	}
	if got := commandNameFromPath("bare.md"); got != "bare" {
		t.Errorf("want bare, got %q", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine(""); got != "" {
		t.Errorf("empty body: want empty, got %q", got)
	}
	if got := firstLine("# comment\n\nfirst real\nsecond"); got != "first real" {
		t.Errorf("want 'first real', got %q", got)
	}
	if got := firstLine("# only comment"); got != "" {
		t.Errorf("comment-only body: want empty, got %q", got)
	}
}
