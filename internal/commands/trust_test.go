package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempTrustStore points trustFilePathFunc at a fresh tempdir for the
// duration of a single test. Returns the file path so tests can inspect or
// pre-populate it.
func withTempTrustStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".arara", trustFileName)

	original := trustFilePathFunc
	trustFilePathFunc = func() string { return path }
	t.Cleanup(func() { trustFilePathFunc = original })

	return path
}

func TestIsTrusted_FalseWhenStoreMissing(t *testing.T) {
	withTempTrustStore(t)

	trusted, err := IsTrusted(t.TempDir())
	if err != nil {
		t.Fatalf("IsTrusted: %v", err)
	}
	if trusted {
		t.Errorf("expected untrusted when store does not exist")
	}
}

func TestTrust_PersistsAndIsTrusted(t *testing.T) {
	storePath := withTempTrustStore(t)
	commandDir := t.TempDir()

	if err := Trust(commandDir); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	trusted, err := IsTrusted(commandDir)
	if err != nil {
		t.Fatalf("IsTrusted: %v", err)
	}
	if !trusted {
		t.Errorf("dir should be trusted after Trust()")
	}

	info, statErr := os.Stat(storePath)
	if statErr != nil {
		t.Fatalf("stat store: %v", statErr)
	}
	if info.Mode().Perm() != trustFilePerms {
		t.Errorf("store perms: want %o, got %o", trustFilePerms, info.Mode().Perm())
	}
}

func TestTrust_Idempotent(t *testing.T) {
	withTempTrustStore(t)
	commandDir := t.TempDir()

	for range 3 {
		if err := Trust(commandDir); err != nil {
			t.Fatalf("Trust: %v", err)
		}
	}

	paths, err := TrustedPaths()
	if err != nil {
		t.Fatalf("TrustedPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("expected 1 trusted entry, got %d (%v)", len(paths), paths)
	}
}

func TestTrust_MultipleDirsBothTrusted(t *testing.T) {
	withTempTrustStore(t)
	dirA := t.TempDir()
	dirB := t.TempDir()

	if err := Trust(dirA); err != nil {
		t.Fatalf("trust A: %v", err)
	}
	if err := Trust(dirB); err != nil {
		t.Fatalf("trust B: %v", err)
	}

	paths, err := TrustedPaths()
	if err != nil {
		t.Fatalf("TrustedPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 trusted entries, got %d (%v)", len(paths), paths)
	}
}

func TestLoadTrustStore_HandlesCorruptJSON(t *testing.T) {
	storePath := withTempTrustStore(t)
	if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(storePath, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	_, err := IsTrusted(t.TempDir())
	if err == nil {
		t.Fatal("expected error parsing corrupt store")
	}
	if !strings.Contains(err.Error(), "parse trust store") {
		t.Errorf("error should mention parse: %v", err)
	}
}

func TestCanonicalizeDir_AbsolutizesRelative(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	canonical, err := canonicalizeDir(".")
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}

	if !filepath.IsAbs(canonical) {
		t.Errorf("expected absolute path, got %q", canonical)
	}

	resolvedCwd, _ := filepath.EvalSymlinks(cwd)
	if canonical != resolvedCwd && canonical != cwd {
		t.Errorf("canonical %q does not match cwd %q (or resolved %q)", canonical, cwd, resolvedCwd)
	}
}

func TestSaveTrustStore_OverwritesExisting(t *testing.T) {
	storePath := withTempTrustStore(t)
	dirA := t.TempDir()
	dirB := t.TempDir()

	if err := Trust(dirA); err != nil {
		t.Fatalf("trust A: %v", err)
	}
	if err := Trust(dirB); err != nil {
		t.Fatalf("trust B: %v", err)
	}

	bytes, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	store := trustStore{}
	if err := json.Unmarshal(bytes, &store); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if store.Version != trustFileVersion {
		t.Errorf("version: want %d, got %d", trustFileVersion, store.Version)
	}
	if len(store.Trusted) != 2 {
		t.Errorf("trusted count: want 2, got %d", len(store.Trusted))
	}
}
