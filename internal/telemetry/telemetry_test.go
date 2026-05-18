package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeSender records every batch it received in order. Used for tests
// that need to assert what got shipped.
type fakeSender struct {
	mu       sync.Mutex
	batches  [][]Event
	failNext bool
}

func (sender *fakeSender) Send(events []Event) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.failNext {
		sender.failNext = false
		return errors.New("simulated send failure")
	}
	cloned := make([]Event, len(events))
	copy(cloned, events)
	sender.batches = append(sender.batches, cloned)
	return nil
}

func (sender *fakeSender) totalEvents() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	count := 0
	for _, batch := range sender.batches {
		count += len(batch)
	}
	return count
}

func TestRecorder_DisabledIsNoop(t *testing.T) {
	sender := &fakeSender{}
	recorder := NewRecorder(State{Enabled: false}, sender)

	for i := 0; i < 10; i++ {
		recorder.Record(Event{Command: "send"})
	}
	if recorder.PendingCount() != 0 {
		t.Errorf("disabled recorder should not enqueue, got %d", recorder.PendingCount())
	}
	if err := recorder.Flush(); err != nil {
		t.Errorf("flush of disabled recorder should be nil error, got: %v", err)
	}
	if sender.totalEvents() != 0 {
		t.Errorf("disabled recorder should never call Send, got %d events", sender.totalEvents())
	}
}

func TestRecorder_EnabledRecordsAndFlushes(t *testing.T) {
	sender := &fakeSender{}
	recorder := NewRecorder(State{Enabled: true, AnonymousID: "test-id"}, sender)

	recorder.Record(Event{Command: "send", ExitCode: 0})
	recorder.Record(Event{Command: "logs", ExitCode: 0})

	if recorder.PendingCount() != 2 {
		t.Errorf("want 2 pending, got %d", recorder.PendingCount())
	}

	if err := recorder.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if sender.totalEvents() != 2 {
		t.Errorf("want 2 sent, got %d", sender.totalEvents())
	}
	if recorder.PendingCount() != 0 {
		t.Errorf("flush should drain queue, got %d pending", recorder.PendingCount())
	}
}

func TestRecorder_FillsDefaultFields(t *testing.T) {
	sender := &fakeSender{}
	recorder := NewRecorder(State{Enabled: true, AnonymousID: "anon-xyz"}, sender)

	recorder.Record(Event{Command: "send"})
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}

	if len(sender.batches) != 1 || len(sender.batches[0]) != 1 {
		t.Fatalf("expected 1 batch of 1, got %d batches", len(sender.batches))
	}
	event := sender.batches[0][0]
	if event.AnonymousID != "anon-xyz" {
		t.Errorf("anonymous ID not filled from state: %q", event.AnonymousID)
	}
	if event.Timestamp.IsZero() {
		t.Errorf("timestamp not filled")
	}
	if event.Platform == "" {
		t.Errorf("platform not filled")
	}
	if event.CLIVersion == "" {
		t.Errorf("cli version not filled")
	}
}

func TestRecorder_BatchesOverMaxSize(t *testing.T) {
	sender := &fakeSender{}
	recorder := NewRecorder(State{Enabled: true, AnonymousID: "x"}, sender)

	const total = MaxBatchSize + 5
	for i := 0; i < total; i++ {
		recorder.Record(Event{Command: "send"})
	}

	if err := recorder.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if len(sender.batches) != 2 {
		t.Errorf("expected 2 batches (50+5), got %d", len(sender.batches))
	}
	if sender.totalEvents() != total {
		t.Errorf("expected %d total events, got %d", total, sender.totalEvents())
	}
}

func TestRecorder_BoundsQueueOnSpammyRecord(t *testing.T) {
	sender := &fakeSender{}
	recorder := NewRecorder(State{Enabled: true, AnonymousID: "x"}, sender)

	for i := 0; i < MaxBatchSize*4; i++ {
		recorder.Record(Event{Command: "send"})
	}

	pending := recorder.PendingCount()
	if pending > MaxBatchSize*2 {
		t.Errorf("queue should be bounded to %d, got %d", MaxBatchSize*2, pending)
	}
}

func TestRecorder_FlushDrainsEvenOnSenderError(t *testing.T) {
	sender := &fakeSender{failNext: true}
	recorder := NewRecorder(State{Enabled: true, AnonymousID: "x"}, sender)

	recorder.Record(Event{Command: "send"})
	if err := recorder.Flush(); err == nil {
		t.Fatal("expected error from failing sender")
	}
	if recorder.PendingCount() != 0 {
		t.Errorf("queue should drain even on error, got %d pending", recorder.PendingCount())
	}
}

func TestLoadState_MissingFileMeansDisabled(t *testing.T) {
	state, err := LoadState(t.TempDir())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.Enabled || state.AnonymousID != "" {
		t.Errorf("missing file should yield zero State, got %+v", state)
	}
}

func TestSaveAndLoadState_RoundTrip(t *testing.T) {
	home := t.TempDir()
	original := State{Enabled: true, AnonymousID: "uuid-abc"}

	if err := SaveState(home, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	reloaded, err := LoadState(home)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if reloaded != original {
		t.Errorf("round-trip mismatch: want %+v, got %+v", original, reloaded)
	}

	// idFile mirror should also be written
	idPath := filepath.Join(home, dirName, idFileName)
	body, _ := os.ReadFile(idPath)
	if string(body) != "uuid-abc\n" {
		t.Errorf("id mirror file content unexpected: %q", string(body))
	}
}

func TestSaveState_EnforcesPerms(t *testing.T) {
	home := t.TempDir()
	if err := SaveState(home, State{Enabled: true, AnonymousID: "x"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(stateFilePath(home))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != filePerms {
		t.Errorf("perms: want %o, got %o", filePerms, info.Mode().Perm())
	}
}

func TestEnsureAnonymousID_GeneratesAndPersists(t *testing.T) {
	home := t.TempDir()
	state := State{Enabled: true}

	updated, err := EnsureAnonymousID(home, state)
	if err != nil {
		t.Fatalf("EnsureAnonymousID: %v", err)
	}
	if updated.AnonymousID == "" {
		t.Error("anonymous id should be generated")
	}

	// Round-trip from disk should return the same id
	reloaded, _ := LoadState(home)
	if reloaded.AnonymousID != updated.AnonymousID {
		t.Errorf("persisted id mismatch: want %q, got %q", updated.AnonymousID, reloaded.AnonymousID)
	}
}

func TestEnsureAnonymousID_PreservesExisting(t *testing.T) {
	home := t.TempDir()
	state := State{Enabled: true, AnonymousID: "preexisting-xyz"}

	updated, err := EnsureAnonymousID(home, state)
	if err != nil {
		t.Fatalf("EnsureAnonymousID: %v", err)
	}
	if updated.AnonymousID != "preexisting-xyz" {
		t.Errorf("existing id should be preserved, got %q", updated.AnonymousID)
	}
}

func TestRecorder_NilIsSafe(t *testing.T) {
	var recorder *Recorder
	recorder.Record(Event{})
	if err := recorder.Flush(); err != nil {
		t.Errorf("nil recorder Flush should be nil error, got: %v", err)
	}
	if recorder.PendingCount() != 0 {
		t.Errorf("nil recorder PendingCount should be 0, got %d", recorder.PendingCount())
	}
	if recorder.Enabled() {
		t.Error("nil recorder should not be Enabled")
	}
}

func TestEvent_FieldsRoundTripJSON(t *testing.T) {
	event := Event{
		Timestamp:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Command:     "send",
		ExitCode:    0,
		DurationMs:  287,
		CLIVersion:  "0.2.0",
		Platform:    "darwin/arm64",
		AnonymousID: "uuid-abc",
	}
	sender := &fakeSender{}
	recorder := NewRecorder(State{Enabled: true, AnonymousID: "x"}, sender)
	recorder.Record(event)
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(sender.batches) != 1 || sender.batches[0][0].Command != "send" {
		t.Errorf("round-trip mismatch: %+v", sender.batches)
	}
}
