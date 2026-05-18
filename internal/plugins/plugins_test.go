package plugins

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipIfNotUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("plugin discovery tests rely on POSIX exec bit; skipping on Windows")
	}
}

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeNonExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\necho hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscover_FindsPluginInPath(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	writeExecutable(t, dir, "arara-hello", `if [ "$1" = "--plugin-description" ]; then echo "says hi"; fi`)

	discoverer := NewDiscoverer(dir, nil, nil)
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "hello" {
		t.Errorf("expected name 'hello', got %q", plugins[0].Name)
	}
	if plugins[0].Description != "says hi" {
		t.Errorf("expected description from --plugin-description, got %q", plugins[0].Description)
	}
}

func TestDiscover_SkipsNonExecutable(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	writeNonExecutable(t, dir, "arara-noexec")

	discoverer := NewDiscoverer(dir, nil, nil)
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Errorf("non-executable should be skipped, got %d", len(plugins))
	}
}

func TestDiscover_SkipsWrongPrefix(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	writeExecutable(t, dir, "git-credential-helper", "")
	writeExecutable(t, dir, "arara-foo", "")

	discoverer := NewDiscoverer(dir, nil, nil)
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Name != "foo" {
		t.Errorf("expected only arara-foo, got %+v", plugins)
	}
}

func TestDiscover_DedupesAcrossDirectories(t *testing.T) {
	skipIfNotUnix(t)
	dirA := t.TempDir()
	dirB := t.TempDir()

	writeExecutable(t, dirA, "arara-foo", "")
	writeExecutable(t, dirB, "arara-foo", "")

	pathEnv := dirA + string(os.PathListSeparator) + dirB
	discoverer := NewDiscoverer(pathEnv, nil, nil)

	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Errorf("first occurrence should win, got %d plugins", len(plugins))
	}
	if !strings.HasPrefix(plugins[0].BinaryPath, dirA) {
		t.Errorf("first occurrence should come from dirA, got %q", plugins[0].BinaryPath)
	}
}

func TestDiscover_HonorsDisabled(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	writeExecutable(t, dir, "arara-foo", "")
	writeExecutable(t, dir, "arara-bar", "")

	discoverer := NewDiscoverer(dir, nil, []string{"foo"})
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Name != "bar" {
		t.Errorf("expected only bar, got %+v", plugins)
	}
}

func TestDiscover_ExtraDirectories(t *testing.T) {
	skipIfNotUnix(t)
	pathDir := t.TempDir()
	extraDir := t.TempDir()

	writeExecutable(t, pathDir, "arara-from-path", "")
	writeExecutable(t, extraDir, "arara-from-extra", "")

	discoverer := NewDiscoverer(pathDir, []string{extraDir}, nil)
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		names = append(names, plugin.Name)
	}
	if len(names) != 2 || !contains(names, "from-path") || !contains(names, "from-extra") {
		t.Errorf("expected from-path and from-extra, got %v", names)
	}
}

func TestDiscover_MissingDirectoryIsIgnored(t *testing.T) {
	discoverer := NewDiscoverer("/definitely/not/a/path", nil, nil)
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected zero plugins, got %d", len(plugins))
	}
}

func TestDiscover_DescriptionFailureFallsBackEmpty(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	writeExecutable(t, dir, "arara-foo", `exit 1`)

	discoverer := NewDiscoverer(dir, nil, nil)
	plugins, err := discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Description != "" {
		t.Errorf("expected empty description on probe failure, got %q", plugins[0].Description)
	}
}

func TestExecutor_RunsBinaryWithArgs(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	captureFile := filepath.Join(dir, "capture.txt")
	binary := writeExecutable(t, dir, "arara-test", `echo "$@" > "`+captureFile+`"
echo "$ARARA_PROFILE" >> "`+captureFile+`"`)

	executor := NewExecutor()
	devNull, _ := os.Open(os.DevNull)
	defer devNull.Close()
	executor.Stdin = devNull
	executor.Stdout = devNull
	executor.Stderr = devNull
	executor.GlobalEnv = map[string]string{"ARARA_PROFILE": "prod"}

	plugin := Plugin{Name: "test", BinaryPath: binary}
	if err := executor.Execute(context.Background(), plugin, []string{"hello", "world"}); err != nil {
		t.Fatal(err)
	}

	captured, _ := os.ReadFile(captureFile)
	if !strings.Contains(string(captured), "hello world") {
		t.Errorf("expected args propagated, got %q", string(captured))
	}
	if !strings.Contains(string(captured), "prod") {
		t.Errorf("expected ARARA_PROFILE=prod, got %q", string(captured))
	}
}

func TestExecutor_ExitErrorIsPropagated(t *testing.T) {
	skipIfNotUnix(t)
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "arara-fail", `exit 13`)

	executor := NewExecutor()
	devNull, _ := os.Open(os.DevNull)
	defer devNull.Close()
	executor.Stdin = devNull
	executor.Stdout = devNull
	executor.Stderr = devNull

	plugin := Plugin{Name: "fail", BinaryPath: binary}
	err := executor.Execute(context.Background(), plugin, nil)
	if err == nil || !strings.Contains(err.Error(), "exited with code 13") {
		t.Errorf("expected exit code 13 in error, got: %v", err)
	}
}

func TestExecutor_RejectsEmptyBinaryPath(t *testing.T) {
	executor := NewExecutor()
	if err := executor.Execute(context.Background(), Plugin{Name: "x"}, nil); err == nil {
		t.Error("expected error on empty binary path")
	}
}

func TestExpandPath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	got, err := ExpandPath("~/bin/x")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(tempHome, "bin", "x") {
		t.Errorf("home expand failed: %q", got)
	}

	got, err = ExpandPath("./relative")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute, got %q", got)
	}

	if _, err := ExpandPath("   "); err == nil {
		t.Error("expected error on empty path")
	}

	got, err = ExpandPath("/absolute")
	if err != nil || got != "/absolute" {
		t.Errorf("absolute pass-through failed: (%q,%v)", got, err)
	}
}

func TestSplitPathList(t *testing.T) {
	if got := splitPathList(""); got != nil {
		t.Errorf("empty input should yield nil, got %v", got)
	}
	separator := string(os.PathListSeparator)
	got := splitPathList("/a" + separator + "  " + separator + "/b")
	if len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Errorf("expected [/a /b], got %v", got)
	}
}

func TestMergeEnv(t *testing.T) {
	base := []string{"A=1", "B=2"}
	got := mergeEnv(base, map[string]string{"C": "3"})
	if len(got) != 3 || got[2] != "C=3" {
		t.Errorf("unexpected merged env: %v", got)
	}

	if same := mergeEnv(base, nil); !sliceEqual(same, base) {
		t.Errorf("nil extras should pass-through, got %v", same)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
