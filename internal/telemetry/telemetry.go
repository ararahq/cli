// Package telemetry collects opt-in usage metrics from the CLI and ships
// them in batches to the AraraHQ backend. The design rules are short:
//
//   - **Off by default.** No event is recorded until the user runs
//     `arara telemetry on`. Their consent is persisted on disk.
//   - **No PII.** We never capture phone numbers, message bodies, recipient
//     names, template content, or API keys. The schema is intentionally
//     small (command name, exit code, latency, version, platform).
//   - **Anonymous.** A UUID v4 generated once and stored locally identifies
//     the install, not the user. Not joinable to any AraraHQ account.
//   - **Fire and forget.** Telemetry never blocks the user — failures to
//     enqueue or flush are silent except in verbose mode.
package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxBatchSize must match the backend cap (CliTelemetryService.MAX_BATCH_SIZE).
	MaxBatchSize = 50

	// DefaultIngestPath is the API path the events go to. Joined to the
	// active client's BaseURL.
	DefaultIngestPath = "/v1/cli/telemetry/events"

	dirName        = ".arara"
	configFileName = "telemetry.json"
	idFileName     = "telemetry-id"
	dirPerms       = 0o700
	filePerms      = 0o600
)

// Event is the on-the-wire record. Field names match the backend
// CliTelemetryEventDto exactly so the JSON encoding round-trips unchanged.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Command     string    `json:"command"`
	ExitCode    int       `json:"exitCode"`
	DurationMs  int       `json:"durationMs"`
	CLIVersion  string    `json:"cliVersion"`
	Platform    string    `json:"platform"`
	AnonymousID string    `json:"anonymousId"`
}

// State is the persisted opt-in flag + anonymous id pointer. Stored as
// telemetry.json next to the rest of the CLI config.
type State struct {
	Enabled     bool   `json:"enabled"`
	AnonymousID string `json:"anonymousId"`
}

// Recorder accumulates events in memory and flushes them in batches to the
// configured Sender. Safe for concurrent use — each Cobra command may end
// up triggering a Record concurrently with a background flush.
type Recorder struct {
	mu       sync.Mutex
	events   []Event
	sender   Sender
	state    State
	disabled bool
}

// Sender is what actually ships a batch over the network. Implemented by
// HTTPSender in production and replaced by a fake in tests so we don't
// need an httptest server for every test that touches Record.
type Sender interface {
	Send(events []Event) error
}

// NewRecorder builds a Recorder honoring the persisted opt-in state. When
// the user has not opted in, every Record call is a no-op and Flush sends
// nothing.
func NewRecorder(state State, sender Sender) *Recorder {
	return &Recorder{
		sender:   sender,
		state:    state,
		disabled: !state.Enabled,
	}
}

// Record enqueues a single event. Drops it silently when telemetry is off,
// or when the queue is full. We never apply backpressure to the calling
// command — the user shouldn't pay latency for telemetry.
func (recorder *Recorder) Record(event Event) {
	if recorder == nil || recorder.disabled {
		return
	}
	if event.AnonymousID == "" {
		event.AnonymousID = recorder.state.AnonymousID
	}
	if event.CLIVersion == "" {
		event.CLIVersion = "dev"
	}
	if event.Platform == "" {
		event.Platform = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.events) >= MaxBatchSize*2 {
		// Bound the queue so a long-running command (REPL, mcp) doesn't
		// leak memory. Drop the oldest to keep the most recent context.
		recorder.events = recorder.events[len(recorder.events)-MaxBatchSize:]
	}
	recorder.events = append(recorder.events, event)
}

// Flush sends every queued event, splitting into batches of MaxBatchSize.
// Errors from Sender are returned but the queue is drained either way —
// retrying the same batch indefinitely creates worse problems than dropping.
func (recorder *Recorder) Flush() error {
	if recorder == nil || recorder.disabled {
		return nil
	}

	recorder.mu.Lock()
	pending := recorder.events
	recorder.events = nil
	recorder.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	var firstErr error
	for start := 0; start < len(pending); start += MaxBatchSize {
		end := start + MaxBatchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[start:end]
		if sendErr := recorder.sender.Send(batch); sendErr != nil && firstErr == nil {
			firstErr = sendErr
		}
	}
	return firstErr
}

// PendingCount returns the in-memory queue size — useful for tests and
// for verbose-mode logging at command exit.
func (recorder *Recorder) PendingCount() int {
	if recorder == nil {
		return 0
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return len(recorder.events)
}

// Enabled reports whether the user opted in. Cobra wrappers use this to
// gate per-command timing instrumentation when telemetry is off.
func (recorder *Recorder) Enabled() bool {
	return recorder != nil && !recorder.disabled
}

// LoadState reads telemetry.json. Missing file = telemetry off (zero State).
func LoadState(homeDir string) (State, error) {
	path := stateFilePath(homeDir)
	bytes, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read telemetry state: %w", readErr)
	}

	var state State
	if unmarshalErr := json.Unmarshal(bytes, &state); unmarshalErr != nil {
		return State{}, fmt.Errorf("parse telemetry state: %w", unmarshalErr)
	}
	return state, nil
}

// SaveState persists opt-in + anonymous id atomically.
func SaveState(homeDir string, state State) error {
	dir := stateDirPath(homeDir)
	if mkErr := os.MkdirAll(dir, dirPerms); mkErr != nil {
		return fmt.Errorf("create telemetry dir: %w", mkErr)
	}

	encoded, marshalErr := json.MarshalIndent(state, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("encode telemetry state: %w", marshalErr)
	}

	if writeErr := writeStateAtomic(dir, stateFilePath(homeDir), encoded); writeErr != nil {
		return writeErr
	}

	// Mirror the anonymous id into a standalone file too — some operators
	// want a single short file they can grep without parsing JSON.
	idPath := filepath.Join(dir, idFileName)
	_ = os.WriteFile(idPath, []byte(state.AnonymousID+"\n"), filePerms)
	return nil
}

// writeStateAtomic writes `encoded` to a sibling temp file in `dir`, chmods
// to filePerms, and renames it on top of `finalPath`. Failures clean up the
// temp file so the directory doesn't accumulate `.telemetry.*.tmp` debris.
func writeStateAtomic(dir, finalPath string, encoded []byte) error {
	tempFile, tempErr := os.CreateTemp(dir, ".telemetry.*.tmp")
	if tempErr != nil {
		return fmt.Errorf("create temp: %w", tempErr)
	}
	tempPath := tempFile.Name()

	if _, writeErr := tempFile.Write(encoded); writeErr != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp: %w", writeErr)
	}
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp: %w", closeErr)
	}
	if chmodErr := os.Chmod(tempPath, filePerms); chmodErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("chmod temp: %w", chmodErr)
	}
	if renameErr := os.Rename(tempPath, finalPath); renameErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename: %w", renameErr)
	}
	return nil
}

// EnsureAnonymousID returns the existing id from state or generates a new
// UUID v4 and persists it. Deterministic per install — never re-rolled.
func EnsureAnonymousID(homeDir string, state State) (State, error) {
	if state.AnonymousID != "" {
		return state, nil
	}
	state.AnonymousID = uuid.NewString()
	if saveErr := SaveState(homeDir, state); saveErr != nil {
		return state, saveErr
	}
	return state, nil
}

func stateDirPath(homeDir string) string {
	return filepath.Join(homeDir, dirName)
}

func stateFilePath(homeDir string) string {
	return filepath.Join(stateDirPath(homeDir), configFileName)
}
