package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipIfNotUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-based execute tests rely on POSIX; skipping on Windows")
	}
}

// withFakeStdinTTY toggles isStdinTTY for a single test, restoring the
// original on cleanup.
func withFakeStdinTTY(t *testing.T, isTTY bool) {
	t.Helper()
	original := isStdinTTY
	isStdinTTY = func() bool { return isTTY }
	t.Cleanup(func() { isStdinTTY = original })
}

// projectCommandFromTempDir synthesizes a project-scope command whose
// SourcePath lives in a freshly created temp directory.
func projectCommandFromTempDir(t *testing.T, body string) (Command, string) {
	t.Helper()
	dir := t.TempDir()
	commandFile := filepath.Join(dir, "echo.md")
	if err := os.WriteFile(commandFile, []byte(body), 0o600); err != nil {
		t.Fatalf("write command file: %v", err)
	}
	return Command{
		Name:       "echo",
		Body:       body,
		Source:     SourceProject,
		SourcePath: commandFile,
	}, dir
}

func TestInterpolate_PositionalArgs(t *testing.T) {
	cases := []struct {
		name string
		body string
		args []string
		want string
	}{
		{"no placeholders", "echo hi", nil, "echo hi"},
		{"single positional", "echo $1", []string{"hello"}, "echo hello"},
		{"missing positional becomes empty", "echo $1 $2", []string{"only"}, "echo only ''"},
		{"join all args", "echo $@", []string{"a", "b c"}, "echo a 'b c'"},
		{"literal dollar via $$", "echo $$1", []string{"ignored"}, "echo $1"},
		{"shell-quoted with spaces", "deploy $1", []string{"my project"}, "deploy 'my project'"},
		{"shell-quoted with single quote", "say $1", []string{"it's"}, `say 'it'\''s'`},
		{"empty body", "", []string{"x"}, ""},
		{"bare alphanumeric not quoted", "go $1", []string{"abc123"}, "go abc123"},
		{"$ at end of body", "echo $", nil, "echo $"},
		{"non-positional after $", "echo $X", nil, "echo $X"},
		{"empty arg joins as ''", "echo $@", []string{""}, "echo ''"},
		{"no args yields empty $@", "echo $@", nil, "echo "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Interpolate(tc.body, tc.args)
			if got != tc.want {
				t.Errorf("Interpolate(%q, %v): want %q, got %q", tc.body, tc.args, tc.want, got)
			}
		})
	}
}

func TestQuoteOne(t *testing.T) {
	cases := map[string]string{
		"":                "''",
		"plain":           "plain",
		"alphanumeric123": "alphanumeric123",
		"with-dash":       "with-dash",
		"with.dot":        "with.dot",
		"path/to/file":    "path/to/file",
		"with space":      "'with space'",
		"it's":            `'it'\''s'`,
		"semicolon;":      "'semicolon;'",
		"$shellvar":       "'$shellvar'",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := quoteOne(input); got != want {
				t.Errorf("quoteOne(%q): want %q, got %q", input, want, got)
			}
		})
	}
}

func TestExecute_Success(t *testing.T) {
	skipIfNotUnix(t)

	stdout := &bytes.Buffer{}
	command := Command{Name: "echo-test", Body: `echo "hello $1"`}
	exitCode, err := Execute(context.Background(), command, ExecOptions{
		Args:   []string{"world"},
		Stdout: stdout,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit: want 0, got %d", exitCode)
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello world" {
		t.Errorf("stdout: want %q, got %q", "hello world", got)
	}
}

func TestExecute_NonZeroExit(t *testing.T) {
	skipIfNotUnix(t)

	command := Command{Name: "fail", Body: "exit 7"}
	exitCode, err := Execute(context.Background(), command, ExecOptions{})
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if exitCode != 7 {
		t.Errorf("exit: want 7, got %d", exitCode)
	}
	if !strings.Contains(err.Error(), "exited with 7") {
		t.Errorf("error should mention exit code, got: %v", err)
	}
}

func TestExecute_CapturesStderr(t *testing.T) {
	skipIfNotUnix(t)

	stderr := &bytes.Buffer{}
	command := Command{Name: "stderr-test", Body: `echo "noisy" 1>&2`}
	_, err := Execute(context.Background(), command, ExecOptions{Stderr: stderr})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "noisy") {
		t.Errorf("stderr capture failed: %q", stderr.String())
	}
}

func TestExecute_HonorsCwd(t *testing.T) {
	skipIfNotUnix(t)

	workDir := t.TempDir()
	stdout := &bytes.Buffer{}
	command := Command{Name: "pwd-test", Body: "pwd", Cwd: workDir}

	if _, err := Execute(context.Background(), command, ExecOptions{Stdout: stdout}); err != nil {
		t.Fatal(err)
	}

	// macOS resolves /var to /private/var; use EvalSymlinks for parity.
	resolvedExpected, _ := filepath.EvalSymlinks(workDir)
	resolvedActual, _ := filepath.EvalSymlinks(strings.TrimSpace(stdout.String()))
	if resolvedActual != resolvedExpected {
		t.Errorf("cwd: want %q, got %q", resolvedExpected, resolvedActual)
	}
}

func TestExecute_PassesEnv(t *testing.T) {
	skipIfNotUnix(t)

	stdout := &bytes.Buffer{}
	command := Command{Name: "env-test", Body: `echo "$ARARA_PROFILE"`}
	_, err := Execute(context.Background(), command, ExecOptions{
		Stdout: stdout,
		Env:    append(os.Environ(), "ARARA_PROFILE=prod"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "prod" {
		t.Errorf("env propagation failed: %q", got)
	}
}

func TestExecute_ContextCancel(t *testing.T) {
	skipIfNotUnix(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	command := Command{Name: "slow", Body: "sleep 5"}
	_, err := Execute(ctx, command, ExecOptions{})
	if err == nil {
		t.Error("expected error from canceled context")
	}
}

func TestPickShell_Override(t *testing.T) {
	gotCmd, gotFlag := pickShell([]string{"/bin/bash", "-c"})
	if gotCmd != "/bin/bash" || gotFlag != "-c" {
		t.Errorf("override not used: got (%q, %q)", gotCmd, gotFlag)
	}
}

func TestPickShell_PlatformDefault(t *testing.T) {
	gotCmd, gotFlag := pickShell(nil)
	if runtime.GOOS == "windows" {
		if gotCmd != windowsShell || gotFlag != windowsShellFlag {
			t.Errorf("windows default mismatch: (%q, %q)", gotCmd, gotFlag)
		}
		return
	}
	if gotCmd != posixShell || gotFlag != posixShellFlag {
		t.Errorf("posix default mismatch: (%q, %q)", gotCmd, gotFlag)
	}
}

func TestExecute_UntrustedProjectNonTTYReturnsErrUntrusted(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, false)

	command, _ := projectCommandFromTempDir(t, `echo "should not run"`)
	stdout := &bytes.Buffer{}

	exitCode, err := Execute(context.Background(), command, ExecOptions{Stdout: stdout})
	if err == nil {
		t.Fatal("expected ErrUntrusted")
	}
	if !errors.Is(err, ErrUntrusted) {
		t.Errorf("expected ErrUntrusted, got %v", err)
	}
	if exitCode != 1 {
		t.Errorf("untrusted exit code: want 1, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty when execution is blocked, got %q", stdout.String())
	}
}

func TestExecute_TrustedProjectRuns(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, false)

	command, commandDir := projectCommandFromTempDir(t, `echo "ran"`)
	if err := Trust(commandDir); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	stdout := &bytes.Buffer{}
	if _, err := Execute(context.Background(), command, ExecOptions{Stdout: stdout}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "ran" {
		t.Errorf("stdout: want %q, got %q", "ran", got)
	}
}

func TestExecute_TrustOptionAutoApproves(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, false)

	command, commandDir := projectCommandFromTempDir(t, `echo "auto"`)

	stdout := &bytes.Buffer{}
	if _, err := Execute(context.Background(), command, ExecOptions{Stdout: stdout, Trust: true}); err != nil {
		t.Fatalf("Execute with Trust: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "auto" {
		t.Errorf("stdout: want %q, got %q", "auto", got)
	}

	trusted, err := IsTrusted(commandDir)
	if err != nil {
		t.Fatalf("IsTrusted: %v", err)
	}
	if !trusted {
		t.Error("Trust: true should persist approval to the trust store")
	}
}

func TestExecute_PromptAlwaysPersistsAndRuns(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, true)

	command, commandDir := projectCommandFromTempDir(t, `echo "prompted"`)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := Execute(context.Background(), command, ExecOptions{
		Stdout:        stdout,
		Stderr:        stderr,
		TrustPromptIn: strings.NewReader("a\n"),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "prompted" {
		t.Errorf("stdout: want %q, got %q", "prompted", got)
	}
	if !strings.Contains(stderr.String(), "Trust commands") {
		t.Errorf("stderr should contain prompt, got %q", stderr.String())
	}

	trusted, _ := IsTrusted(commandDir)
	if !trusted {
		t.Error("'a' reply should persist trust")
	}
}

func TestExecute_PromptYesRunsWithoutPersisting(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, true)

	command, commandDir := projectCommandFromTempDir(t, `echo "once"`)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := Execute(context.Background(), command, ExecOptions{
		Stdout:        stdout,
		Stderr:        stderr,
		TrustPromptIn: strings.NewReader("y\n"),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "once" {
		t.Errorf("stdout: want %q, got %q", "once", got)
	}

	trusted, _ := IsTrusted(commandDir)
	if trusted {
		t.Error("'y' reply must NOT persist trust")
	}
}

func TestExecute_PromptDeniedAborts(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, true)

	command, _ := projectCommandFromTempDir(t, `echo "denied"`)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := Execute(context.Background(), command, ExecOptions{
		Stdout:        stdout,
		Stderr:        stderr,
		TrustPromptIn: strings.NewReader("n\n"),
	})
	if err == nil {
		t.Fatal("expected ErrUntrusted on denial")
	}
	if !errors.Is(err, ErrUntrusted) {
		t.Errorf("expected ErrUntrusted, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", stdout.String())
	}
}

func TestExecute_UserScopeSkipsTrustCheck(t *testing.T) {
	skipIfNotUnix(t)
	withTempTrustStore(t)
	withFakeStdinTTY(t, false)

	command := Command{
		Name:       "user-cmd",
		Body:       `echo "user-scope"`,
		Source:     SourceUser,
		SourcePath: filepath.Join(t.TempDir(), "u.md"),
	}

	stdout := &bytes.Buffer{}
	if _, err := Execute(context.Background(), command, ExecOptions{Stdout: stdout}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "user-scope" {
		t.Errorf("stdout: want %q, got %q", "user-scope", got)
	}
}

func TestIsBareWord(t *testing.T) {
	bareWords := []string{"abc", "x-y", "x_y", "x.y", "x/y", "x:y", "x@y", "x+y", "123"}
	for _, word := range bareWords {
		if !isBareWord(word) {
			t.Errorf("%q should be bare", word)
		}
	}
	notBareWords := []string{"a b", "a;b", "a'b", "a\"b", "a$b", "a&b"}
	for _, word := range notBareWords {
		if isBareWord(word) {
			t.Errorf("%q should NOT be bare", word)
		}
	}
}
