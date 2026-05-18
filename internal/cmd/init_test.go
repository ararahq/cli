package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldInitProject_FreshDir(t *testing.T) {
	root := t.TempDir()

	report, err := scaffoldInitProject(root, false)
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	for _, expected := range []string{
		filepath.Join(root, ".arara", "settings.json"),
		filepath.Join(root, ".arara", "commands", "deploy.md"),
		filepath.Join(root, ".arara", "hooks", "log-send.sh"),
	} {
		info, statErr := os.Stat(expected)
		if statErr != nil {
			t.Errorf("%s missing: %v", expected, statErr)
			continue
		}
		if info.IsDir() {
			t.Errorf("%s is dir, want file", expected)
		}
	}

	for _, entry := range report {
		if !entry.Created {
			t.Errorf("expected all entries created on fresh dir, got skip: %+v", entry)
		}
	}
}

func TestScaffoldInitProject_HookFileIsExecutable(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffoldInitProject(root, false); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	hook := filepath.Join(root, ".arara", "hooks", "log-send.sh")
	info, err := os.Stat(hook)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook should be executable, got mode %v", info.Mode().Perm())
	}
}

func TestScaffoldInitProject_PreservesExistingWithoutForce(t *testing.T) {
	root := t.TempDir()

	// Pre-create settings.json with custom content.
	settingsPath := filepath.Join(root, ".arara", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	customContent := []byte(`{"theme":"mono"}`)
	if err := os.WriteFile(settingsPath, customContent, 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := scaffoldInitProject(root, false)
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	got, _ := os.ReadFile(settingsPath)
	if string(got) != string(customContent) {
		t.Errorf("existing settings.json overwritten: got %q", string(got))
	}

	skipFound := false
	for _, entry := range report {
		if strings.HasSuffix(entry.Path, "settings.json") && !entry.Created {
			skipFound = true
		}
	}
	if !skipFound {
		t.Error("expected report to mark settings.json as skipped")
	}
}

func TestScaffoldInitProject_OverwritesWithForce(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, ".arara", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := scaffoldInitProject(root, true); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	got, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(got), "$schema") {
		t.Errorf("expected template content with --force, got %q", string(got))
	}
}

func TestUpdateGitignore_AppendsWhenAbsent(t *testing.T) {
	root := t.TempDir()
	gitignorePath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := updateGitignore(root)
	if err != nil {
		t.Fatalf("updateGitignore: %v", err)
	}
	if entry == nil || !entry.Created {
		t.Fatalf("expected created entry, got %+v", entry)
	}

	got, _ := os.ReadFile(gitignorePath)
	if !strings.Contains(string(got), ".arara/credentials") {
		t.Errorf("expected .arara/credentials in gitignore, got:\n%s", string(got))
	}
	if !strings.Contains(string(got), "node_modules/") {
		t.Error("existing entries should be preserved")
	}
}

func TestUpdateGitignore_SkipsWhenAlreadyMentioned(t *testing.T) {
	root := t.TempDir()
	gitignorePath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".arara/credentials.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := updateGitignore(root)
	if err != nil {
		t.Fatalf("updateGitignore: %v", err)
	}
	if entry == nil || entry.Created {
		t.Errorf("expected non-created entry when already present, got %+v", entry)
	}
}

func TestUpdateGitignore_NoOpWhenFileMissing(t *testing.T) {
	root := t.TempDir()

	entry, err := updateGitignore(root)
	if err != nil {
		t.Fatalf("updateGitignore: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil entry when no .gitignore exists, got %+v", entry)
	}
}

func TestScaffoldInitProject_IsIdempotent(t *testing.T) {
	root := t.TempDir()

	if _, err := scaffoldInitProject(root, false); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	report, err := scaffoldInitProject(root, false)
	if err != nil {
		t.Fatalf("second scaffold: %v", err)
	}

	for _, entry := range report {
		if entry.Created {
			t.Errorf("second scaffold should skip everything, got created: %+v", entry)
		}
	}
}

func TestRunInit_TargetsDirFlag(t *testing.T) {
	originalDir := initDirFlag
	originalForce := initForceFlag
	t.Cleanup(func() {
		initDirFlag = originalDir
		initForceFlag = originalForce
	})

	target := t.TempDir()
	initDirFlag = target
	initForceFlag = false

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	settings := filepath.Join(target, ".arara", "settings.json")
	if _, err := os.Stat(settings); err != nil {
		t.Errorf("settings.json should exist after runInit: %v", err)
	}
}
