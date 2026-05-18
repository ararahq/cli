package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ararahq/cli/internal/telemetry"
)

func TestRunTelemetryOn_PersistsEnabledStateAndID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := runTelemetryOn(nil, nil); err != nil {
		t.Fatalf("runTelemetryOn: %v", err)
	}

	state, err := telemetry.LoadState(home)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !state.Enabled {
		t.Error("state should be enabled after on")
	}
	if state.AnonymousID == "" {
		t.Error("anonymous id should be generated")
	}
}

func TestRunTelemetryOff_PersistsDisabledState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// First enable
	if err := runTelemetryOn(nil, nil); err != nil {
		t.Fatal(err)
	}

	if err := runTelemetryOff(nil, nil); err != nil {
		t.Fatalf("runTelemetryOff: %v", err)
	}

	state, _ := telemetry.LoadState(home)
	if state.Enabled {
		t.Error("state should be disabled after off")
	}
	if state.AnonymousID == "" {
		t.Error("anonymous id should be preserved across off (no need to re-roll)")
	}
}

func TestRunTelemetryStatus_PrintsState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := telemetry.SaveState(home, telemetry.State{Enabled: true, AnonymousID: "abc-123"}); err != nil {
		t.Fatal(err)
	}

	if err := runTelemetryStatus(nil, nil); err != nil {
		t.Errorf("runTelemetryStatus: %v", err)
	}
}

func TestPrintTelemetryStatus_EnabledRenders(t *testing.T) {
	buffer := &bytes.Buffer{}
	state := telemetry.State{Enabled: true, AnonymousID: "anon-xyz"}

	if err := printTelemetryStatus(buffer, state); err != nil {
		t.Fatal(err)
	}

	rendered := buffer.String()
	if !strings.Contains(rendered, "enabled") {
		t.Errorf("output should contain 'enabled', got: %q", rendered)
	}
	if !strings.Contains(rendered, "anon-xyz") {
		t.Errorf("output should contain anonymous id, got: %q", rendered)
	}
}

func TestPrintTelemetryStatus_DisabledRenders(t *testing.T) {
	buffer := &bytes.Buffer{}
	if err := printTelemetryStatus(buffer, telemetry.State{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "disabled") {
		t.Errorf("output should contain 'disabled', got: %q", buffer.String())
	}
}

func TestRunTelemetryOn_OverwritesExistingId(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-populate with a known id and disabled state
	preexisting := telemetry.State{Enabled: false, AnonymousID: "preexisting-id"}
	if err := telemetry.SaveState(home, preexisting); err != nil {
		t.Fatal(err)
	}

	if err := runTelemetryOn(nil, nil); err != nil {
		t.Fatalf("runTelemetryOn: %v", err)
	}

	state, _ := telemetry.LoadState(home)
	if !state.Enabled {
		t.Error("should be enabled after on")
	}
	if state.AnonymousID != "preexisting-id" {
		t.Errorf("existing id should be preserved across on, got %q", state.AnonymousID)
	}
}

func TestUserHomeDir_ReturnsValue(t *testing.T) {
	home, err := userHomeDir()
	if err != nil {
		t.Fatalf("userHomeDir: %v", err)
	}
	if home == "" {
		t.Error("home should not be empty")
	}
}

func TestResolveTelemetryRecorder_NilWhenStateMissing(t *testing.T) {
	// Reset the singleton — sync.Once means tests can race. Use a new
	// HOME so LoadState returns zero value.
	resetTelemetryRecorderSingleton(t)
	t.Setenv("HOME", t.TempDir())

	got := resolveTelemetryRecorder()
	if got.Enabled() {
		t.Error("recorder should not be enabled when state file missing")
	}
}

// resetTelemetryRecorderSingleton clears the cached recorder so tests can
// drive resolveTelemetryRecorder with a fresh state. Restores on cleanup.
//
// We can't copy a sync.Once value (vet rule), so we just zero out the
// recorder pointer and replace the Once with a freshly-zero one. The
// cleanup path does the same — anything that previously ran one-shot
// initialization is allowed to run again in the next test that needs it.
func resetTelemetryRecorderSingleton(t *testing.T) {
	t.Helper()
	originalInst := telemetryRecorderInst
	t.Cleanup(func() {
		telemetryRecorderInst = originalInst
		telemetryRecorderOnce = sync.Once{}
	})
	telemetryRecorderInst = nil
	telemetryRecorderOnce = sync.Once{}
}

// silence unused-import warning if filepath/os not referenced elsewhere
var (
	_ = os.UserHomeDir
	_ = filepath.Join
)
