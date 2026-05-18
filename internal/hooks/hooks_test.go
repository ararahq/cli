package hooks

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeShellScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	scriptPath := filepath.Join(dir, name)
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return scriptPath
}

func skipIfNotUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hook tests rely on bash; skipping on Windows")
	}
}

func TestRunner_NoScripts_NoOp(t *testing.T) {
	runner := NewRunner()
	outcomes, err := runner.Run(context.Background(), nil, Event{Name: EventPreSend})
	if err != nil {
		t.Errorf("expected no error for empty scripts, got: %v", err)
	}
	if len(outcomes) != 0 {
		t.Errorf("expected zero outcomes, got %d", len(outcomes))
	}
}

func TestRunner_PreSend_Aborts(t *testing.T) {
	skipIfNotUnix(t)

	dir := t.TempDir()
	script := writeShellScript(t, dir, "abort.sh", `echo "blocking" 1>&2; exit 7`)

	runner := NewRunner()
	stderr := &bytes.Buffer{}
	runner.Stderr = stderr

	outcomes, err := runner.Run(context.Background(), []string{script}, Event{Name: EventPreSend})
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("expected ErrAborted, got: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].ExitCode != 7 {
		t.Errorf("expected exit code 7, got %d", outcomes[0].ExitCode)
	}
	if !strings.Contains(outcomes[0].Stderr, "blocking") {
		t.Errorf("expected captured stderr, got %q", outcomes[0].Stderr)
	}
}

func TestRunner_PostDeliver_DoesNotAbort(t *testing.T) {
	skipIfNotUnix(t)

	dir := t.TempDir()
	failing := writeShellScript(t, dir, "fail.sh", `exit 1`)
	succeeding := writeShellScript(t, dir, "ok.sh", `exit 0`)

	runner := NewRunner()
	stderr := &bytes.Buffer{}
	runner.Stderr = stderr

	outcomes, err := runner.Run(context.Background(), []string{failing, succeeding}, Event{Name: EventPostDeliver})
	if err != nil {
		t.Errorf("postDeliver should not abort, got: %v", err)
	}
	if len(outcomes) != 2 {
		t.Errorf("expected both hooks to run, got %d outcomes", len(outcomes))
	}
	if !strings.Contains(stderr.String(), "failed") {
		t.Errorf("expected failure log on stderr, got %q", stderr.String())
	}
}

func TestRunner_PassesEventEnvAndPayloadStdin(t *testing.T) {
	skipIfNotUnix(t)

	dir := t.TempDir()
	captureFile := filepath.Join(dir, "capture.txt")
	script := writeShellScript(t, dir, "capture.sh", `echo "$ARARA_EVENT" > "`+captureFile+`"
echo "$ARARA_PROFILE" >> "`+captureFile+`"
cat >> "`+captureFile+`"`)

	runner := NewRunner()
	_, err := runner.Run(context.Background(), []string{script}, Event{
		Name:    EventPreSend,
		Profile: "prod",
		Mode:    "live",
		Payload: map[string]string{"to": "+5511"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	captured, readErr := os.ReadFile(captureFile)
	if readErr != nil {
		t.Fatal(readErr)
	}

	got := string(captured)
	if !strings.Contains(got, EventPreSend) {
		t.Errorf("ARARA_EVENT not propagated: %q", got)
	}
	if !strings.Contains(got, "prod") {
		t.Errorf("ARARA_PROFILE not propagated: %q", got)
	}
	if !strings.Contains(got, `"to":"+5511"`) {
		t.Errorf("payload not on stdin: %q", got)
	}
}

func TestRunner_HookNotFound(t *testing.T) {
	runner := NewRunner()
	outcomes, err := runner.Run(context.Background(), []string{"/definitely/not/a/path/hook.sh"}, Event{Name: EventPostDeliver})
	if err != nil {
		t.Errorf("postDeliver missing hook should not abort, got: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Err == nil {
		t.Errorf("expected outcome with err, got %+v", outcomes)
	}
}

func TestRunner_HookNotFound_PreSendAborts(t *testing.T) {
	runner := NewRunner()
	_, err := runner.Run(context.Background(), []string{"/definitely/not/a/path/hook.sh"}, Event{Name: EventPreSend})
	if !errors.Is(err, ErrAborted) {
		t.Errorf("preSend with missing hook should abort, got: %v", err)
	}
}

func TestRunner_Timeout(t *testing.T) {
	skipIfNotUnix(t)

	dir := t.TempDir()
	script := writeShellScript(t, dir, "slow.sh", `sleep 5`)

	runner := NewRunner()
	runner.Timeout = 100 * time.Millisecond

	outcomes, _ := runner.Run(context.Background(), []string{script}, Event{Name: EventPostDeliver})
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Err == nil || !strings.Contains(outcomes[0].Err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", outcomes[0].Err)
	}
}

func TestExpandPath_HomeRelative(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	got, err := ExpandPath("~/hooks/foo.sh")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tempHome, "hooks", "foo.sh")
	if got != want {
		t.Errorf("ExpandPath: want %q, got %q", want, got)
	}
}

func TestExpandPath_Relative(t *testing.T) {
	got, err := ExpandPath("./hooks/foo.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
}

func TestExpandPath_Absolute(t *testing.T) {
	got, err := ExpandPath("/usr/local/bin/hook")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/local/bin/hook" {
		t.Errorf("absolute path should pass through, got %q", got)
	}
}

func TestExpandPath_Empty(t *testing.T) {
	if _, err := ExpandPath("   "); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestIsAbortableEvent(t *testing.T) {
	abortable := []string{EventPreSend, EventPreCampaign}
	for _, event := range abortable {
		if !isAbortableEvent(event) {
			t.Errorf("%s should be abortable", event)
		}
	}
	nonAbortable := []string{EventPostDeliver, EventPostCampaign, EventOnError, "unknown"}
	for _, event := range nonAbortable {
		if isAbortableEvent(event) {
			t.Errorf("%s should NOT be abortable", event)
		}
	}
}

func TestRunner_ChannelEncodesPayloadJSON(t *testing.T) {
	runner := NewRunner()
	_, err := runner.Run(context.Background(), nil, Event{Payload: make(chan int)})
	if err != nil {
		t.Errorf("nil scripts with bad payload should still no-op, got: %v", err)
	}
}

func TestRunner_PayloadJSONEncodingFails(t *testing.T) {
	skipIfNotUnix(t)

	dir := t.TempDir()
	script := writeShellScript(t, dir, "ok.sh", `exit 0`)

	runner := NewRunner()
	_, err := runner.Run(context.Background(), []string{script}, Event{Payload: make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "serialize hook payload") {
		t.Errorf("expected payload serialization error, got: %v", err)
	}
}
