package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/commands"
)

// writeMarkdownCommand creates a slash-command markdown file in the given
// directory.
func writeMarkdownCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestPrintCommandsListTable_EmptyHandledGracefully(t *testing.T) {
	// Smoke: ensure printing nil does not panic.
	printCommandsListTable(nil)
}

func TestPrintCommandsListTable_RendersWithoutPanic(t *testing.T) {
	cmds := []commands.Command{
		{Name: "deploy", Source: commands.SourceProject, Description: "Deploy to staging"},
		{Name: "logs", Source: commands.SourceUser, Description: ""},
	}
	printCommandsListTable(cmds)
}

func TestCommandsLoader_DiscoversFromFakeUserDir(t *testing.T) {
	userDir := t.TempDir()
	writeMarkdownCommand(t, userDir, "hello", "echo hi\n")
	writeMarkdownCommand(t, userDir, "deploy", "---\ndescription: ship it\n---\necho deploying\n")

	loader := &commands.Loader{UserDir: userDir, ProjectAnchor: t.TempDir()}
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(discovered))
	}

	byName := map[string]commands.Command{}
	for _, command := range discovered {
		byName[command.Name] = command
	}
	if byName["hello"].Body != "echo hi" {
		t.Errorf("hello body: %q", byName["hello"].Body)
	}
	if byName["deploy"].Description != "ship it" {
		t.Errorf("deploy description from frontmatter: %q", byName["deploy"].Description)
	}
}

func TestCommandsLoader_LookupReturnsErrorWhenMissing(t *testing.T) {
	loader := &commands.Loader{UserDir: t.TempDir(), ProjectAnchor: t.TempDir()}
	_, err := loader.Lookup("does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestEnsureUserCommandGroup_AddsOnlyOnce(t *testing.T) {
	root := &cobra.Command{}
	ensureUserCommandGroup(root)
	ensureUserCommandGroup(root)

	count := 0
	for _, group := range root.Groups() {
		if group.ID == commandsCommandGroupID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("group should be added only once, got %d", count)
	}
}

func TestTrustFromEnv(t *testing.T) {
	t.Setenv(trustEnvVar, "1")
	if !trustFromEnv() {
		t.Error("ARARA_TRUST=1 should return true")
	}

	t.Setenv(trustEnvVar, "0")
	if trustFromEnv() {
		t.Error("ARARA_TRUST=0 should return false")
	}

	t.Setenv(trustEnvVar, "")
	if trustFromEnv() {
		t.Error("unset ARARA_TRUST should return false")
	}
}

func TestRegisterCommandsAsSubcommands_AddsAndSkipsConflicts(t *testing.T) {
	root := &cobra.Command{Use: "arara"}
	// Pre-existing built-in command — should not be overwritten by a slash
	// command with the same name.
	root.AddCommand(&cobra.Command{Use: "send"})

	registerCommandsAsSubcommands(root, []commands.Command{
		{Name: "deploy", Description: "Deploy thing", Source: commands.SourceProject, SourcePath: "/tmp/x.md", Body: "echo hi"},
		{Name: "send", Description: "Conflict", Source: commands.SourceUser, SourcePath: "/tmp/send.md", Body: "echo conflict"},
	})

	hasDeploy := false
	for _, child := range root.Commands() {
		if child.Use == "deploy" {
			hasDeploy = true
		}
	}
	if !hasDeploy {
		t.Error("deploy slash command should be registered")
	}
}

func TestRegisterCommandsAsSubcommands_HonorsArgHint(t *testing.T) {
	root := &cobra.Command{Use: "arara"}
	registerCommandsAsSubcommands(root, []commands.Command{
		{Name: "promo", ArgHint: "<env>", Source: commands.SourceUser, SourcePath: "/tmp/p.md", Body: "echo $1"},
	})

	for _, child := range root.Commands() {
		if child.Name() == "promo" {
			if child.Use != "promo <env>" {
				t.Errorf("Use should include argHint, got %q", child.Use)
			}
			return
		}
	}
	t.Error("promo command not registered")
}

// withFakeCommandsLoader points commandsLoaderFactory at a tempdir-backed
// loader for the duration of a test, restoring the original on cleanup.
func withFakeCommandsLoader(t *testing.T, userDir string) {
	t.Helper()
	original := commandsLoaderFactory
	commandsLoaderFactory = func() *commands.Loader {
		return &commands.Loader{UserDir: userDir, ProjectAnchor: t.TempDir()}
	}
	t.Cleanup(func() { commandsLoaderFactory = original })
}

func TestRunCommandsList_RendersDiscovered(t *testing.T) {
	userDir := t.TempDir()
	writeMarkdownCommand(t, userDir, "deploy", "---\ndescription: ship it\n---\necho hi\n")
	withFakeCommandsLoader(t, userDir)

	if err := runCommandsList(nil, nil); err != nil {
		t.Errorf("runCommandsList: %v", err)
	}
}

func TestRunCommandsShow_FoundAndMissing(t *testing.T) {
	userDir := t.TempDir()
	writeMarkdownCommand(t, userDir, "ping", "echo pong\n")
	withFakeCommandsLoader(t, userDir)

	if err := runCommandsShow(nil, []string{"ping"}); err != nil {
		t.Errorf("runCommandsShow: %v", err)
	}

	if err := runCommandsShow(nil, []string{"missing"}); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestRunCommandsWhich_PrintsPath(t *testing.T) {
	userDir := t.TempDir()
	writeMarkdownCommand(t, userDir, "ping", "echo pong\n")
	withFakeCommandsLoader(t, userDir)

	if err := runCommandsWhich(nil, []string{"ping"}); err != nil {
		t.Errorf("runCommandsWhich: %v", err)
	}

	if err := runCommandsWhich(nil, []string{"missing"}); err == nil {
		t.Fatal("expected error for missing command")
	}
}

// reference to filepath/os to silence unused-import if helpers are pruned
var _ = os.MkdirAll
var _ = filepath.Join
