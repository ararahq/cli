package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	trustFileName    = "trusted-commands.json"
	trustFilePerms   = 0o600
	trustDirPerms    = 0o700
	trustFileVersion = 1
)

// ErrUntrusted is returned when execute.go refuses to run a project-scope
// slash command whose source directory has not been approved by the user.
// Callers map this to a non-zero exit code with a remediation message.
var ErrUntrusted = errors.New("slash command source is not trusted")

// trustStore is the on-disk schema for ~/.arara/trusted-commands.json. The
// version field exists so future migrations can detect old layouts without
// blowing up.
type trustStore struct {
	Version int          `json:"version"`
	Trusted []trustEntry `json:"trusted"`
}

type trustEntry struct {
	Path      string    `json:"path"`
	TrustedAt time.Time `json:"trustedAt"`
}

// trustFilePathFunc is overridden in tests to point at a tempdir-backed file
// instead of the real ~/.arara/trusted-commands.json.
var trustFilePathFunc = defaultTrustFilePath

// IsTrusted reports whether the directory containing a slash command file
// has been previously approved by the user. The check is performed against
// the absolute, symlink-resolved form of the path so cosmetic differences
// (./vs absolute, trailing slash) don't bypass the approval.
func IsTrusted(commandDir string) (bool, error) {
	canonical, canonicalErr := canonicalizeDir(commandDir)
	if canonicalErr != nil {
		return false, canonicalErr
	}

	store, loadErr := loadTrustStore()
	if loadErr != nil {
		return false, loadErr
	}

	for _, entry := range store.Trusted {
		if entry.Path == canonical {
			return true, nil
		}
	}
	return false, nil
}

// Trust persists approval for a command directory. It is idempotent: calling
// it twice does not duplicate the entry, but it does refresh trustedAt.
func Trust(commandDir string) error {
	canonical, canonicalErr := canonicalizeDir(commandDir)
	if canonicalErr != nil {
		return canonicalErr
	}

	store, loadErr := loadTrustStore()
	if loadErr != nil {
		return loadErr
	}

	now := time.Now().UTC()
	updated := false
	for index, entry := range store.Trusted {
		if entry.Path == canonical {
			store.Trusted[index].TrustedAt = now
			updated = true
			break
		}
	}
	if !updated {
		store.Trusted = append(store.Trusted, trustEntry{Path: canonical, TrustedAt: now})
	}

	return saveTrustStore(store)
}

// TrustedPaths returns the list of currently approved directories, sorted
// for deterministic output (used by `arara commands trust list` if added).
func TrustedPaths() ([]string, error) {
	store, loadErr := loadTrustStore()
	if loadErr != nil {
		return nil, loadErr
	}

	paths := make([]string, 0, len(store.Trusted))
	for _, entry := range store.Trusted {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	return paths, nil
}

func loadTrustStore() (*trustStore, error) {
	path := trustFilePathFunc()
	bytes, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return &trustStore{Version: trustFileVersion}, nil
		}
		return nil, fmt.Errorf("read trust store: %w", readErr)
	}

	store := &trustStore{}
	if unmarshalErr := json.Unmarshal(bytes, store); unmarshalErr != nil {
		return nil, fmt.Errorf("parse trust store at %s: %w", path, unmarshalErr)
	}
	if store.Version == 0 {
		store.Version = trustFileVersion
	}
	return store, nil
}

func saveTrustStore(store *trustStore) error {
	path := trustFilePathFunc()
	if mkErr := os.MkdirAll(filepath.Dir(path), trustDirPerms); mkErr != nil {
		return fmt.Errorf("create trust store dir: %w", mkErr)
	}

	store.Version = trustFileVersion
	encoded, marshalErr := json.MarshalIndent(store, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("encode trust store: %w", marshalErr)
	}

	tempFile, tempErr := os.CreateTemp(filepath.Dir(path), ".trusted-commands.*.tmp")
	if tempErr != nil {
		return fmt.Errorf("create temp file: %w", tempErr)
	}
	tempPath := tempFile.Name()

	if _, writeErr := tempFile.Write(encoded); writeErr != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp trust store: %w", writeErr)
	}
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp trust store: %w", closeErr)
	}
	if chmodErr := os.Chmod(tempPath, trustFilePerms); chmodErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("chmod temp trust store: %w", chmodErr)
	}
	if renameErr := os.Rename(tempPath, path); renameErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename trust store: %w", renameErr)
	}

	return nil
}

func canonicalizeDir(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("trust path is empty")
	}
	absolute, absErr := filepath.Abs(dir)
	if absErr != nil {
		return "", fmt.Errorf("resolve absolute path: %w", absErr)
	}
	if resolved, evalErr := filepath.EvalSymlinks(absolute); evalErr == nil {
		return resolved, nil
	}
	// EvalSymlinks fails for paths that don't exist on disk yet — fall back
	// to the absolute form so tests and never-touched dirs still work.
	return absolute, nil
}

func defaultTrustFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", UserCommandsDirName, trustFileName)
	}
	return filepath.Join(home, UserCommandsDirName, trustFileName)
}
