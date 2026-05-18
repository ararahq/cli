package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"golang.org/x/term"
)

const (
	posixShell        = "sh"
	posixShellFlag    = "-c"
	windowsShell      = "cmd.exe"
	windowsShellFlag  = "/C"
	literalDollar     = "$$"
	literalDollarMark = "\x00ARARA_LITERAL_DOLLAR\x00"

	trustReplyYes    = "y"
	trustReplyAlways = "a"
)

// isStdinTTY is overridden in tests so we can simulate non-interactive
// invocations (CI, piped stdin) without mucking with file descriptors.
var isStdinTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ExecOptions controls how a slash command body is executed.
type ExecOptions struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string

	// Trust skips the interactive prompt for project-scope commands and
	// persists the source dir to the trust store. Used by --trust on the
	// CLI for unattended runs (CI, scripts).
	Trust bool

	// TrustPromptIn is the reader the interactive trust prompt reads from.
	// Defaults to os.Stdin. Tests can inject a strings.NewReader to drive
	// the y/N/a flow without a real TTY.
	TrustPromptIn io.Reader

	// shellOverride lets tests inject a custom command + flag (e.g., `bash -c`)
	// without touching the production POSIX/Windows branches.
	shellOverride []string
}

// Execute interpolates the body, runs it via the platform shell, and returns
// the resulting exit code (0 on success). A nil error means the command ran
// to completion; the returned int is the underlying exit status.
//
// Project-scope commands run only after the source directory has been
// approved by the user (interactive prompt) or by passing options.Trust.
// User-scope commands skip the trust check — the user controls their own
// home directory.
func Execute(ctx context.Context, command Command, options ExecOptions) (int, error) {
	if trustErr := ensureTrusted(command, options); trustErr != nil {
		return 1, trustErr
	}

	interpolated := Interpolate(command.Body, options.Args)

	shellCmd, shellFlag := pickShell(options.shellOverride)

	cmd := exec.CommandContext(ctx, shellCmd, shellFlag, interpolated)
	cmd.Stdin = orDevNull(options.Stdin)
	cmd.Stdout = orDiscard(options.Stdout)
	cmd.Stderr = orDiscard(options.Stderr)
	cmd.Env = orInheritEnv(options.Env)

	if command.Cwd != "" {
		cmd.Dir = command.Cwd
	}

	runErr := cmd.Run()
	if runErr == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), fmt.Errorf("slash command %q exited with %d", command.Name, exitErr.ExitCode())
	}

	return -1, fmt.Errorf("failed to run slash command %q: %w", command.Name, runErr)
}

// Interpolate substitutes Bash-style positional placeholders into the body:
//
//	$1..$9   → shell-quoted args[i-1]; empty string if missing
//	$@       → joined, individually shell-quoted args
//	$$       → literal $ (so authors can embed $1 verbatim by writing $$1)
//
// Everything else passes through unchanged for the shell to evaluate at run
// time. This matches RFC 0002's guide-level specification.
func Interpolate(body string, args []string) string {
	if body == "" {
		return ""
	}

	// Reserve $$ first so subsequent passes don't see $-pairs.
	withoutLiteralDollar := strings.ReplaceAll(body, literalDollar, literalDollarMark)
	withArgs := interpolatePositional(withoutLiteralDollar, args)
	finalText := strings.ReplaceAll(withArgs, literalDollarMark, "$")
	return finalText
}

func interpolatePositional(body string, args []string) string {
	builder := strings.Builder{}
	i := 0
	for i < len(body) {
		ch := body[i]
		if ch != '$' || i+1 >= len(body) {
			builder.WriteByte(ch)
			i++
			continue
		}

		next := body[i+1]
		switch {
		case next == '@':
			builder.WriteString(quoteAll(args))
			i += 2
		case next >= '1' && next <= '9':
			position := int(next - '0')
			builder.WriteString(quoteOne(positionalArg(args, position)))
			i += 2
		default:
			builder.WriteByte(ch)
			i++
		}
	}
	return builder.String()
}

func positionalArg(args []string, position int) string {
	index := position - 1
	if index < 0 || index >= len(args) {
		return ""
	}
	return args[index]
}

func quoteAll(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, quoteOne(arg))
	}
	return strings.Join(parts, " ")
}

// quoteOne returns a POSIX shell-safe single-quoted version of the value.
// Empty strings become ”, which the shell parses as an empty argument.
func quoteOne(value string) string {
	if value == "" {
		return "''"
	}

	// Fast path: if the value has no shell metacharacters or single quotes,
	// no quoting is needed at all.
	if isBareWord(value) {
		return value
	}

	escaped := strings.ReplaceAll(value, "'", `'\''`)
	return "'" + escaped + "'"
}

func isBareWord(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '_', '-', '.', '+', '/', ':', '@':
			continue
		}
		return false
	}
	return true
}

func pickShell(override []string) (string, string) {
	if len(override) >= 2 {
		return override[0], override[1]
	}
	if runtime.GOOS == "windows" {
		return windowsShell, windowsShellFlag
	}
	return posixShell, posixShellFlag
}

func orDevNull(reader io.Reader) io.Reader {
	if reader != nil {
		return reader
	}
	devNull, _ := os.Open(os.DevNull)
	return devNull
}

func orDiscard(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return io.Discard
}

func orInheritEnv(env []string) []string {
	if env != nil {
		return env
	}
	return os.Environ()
}

// ensureTrusted is the gate Execute calls before running a project-scope
// command. User-scope commands and missing source paths bypass the gate
// (the latter happens in tests that synthesize Command literals).
func ensureTrusted(command Command, options ExecOptions) error {
	if command.Source != SourceProject || command.SourcePath == "" {
		return nil
	}

	commandDir := filepath.Dir(command.SourcePath)

	if options.Trust {
		if trustErr := Trust(commandDir); trustErr != nil {
			return fmt.Errorf("persist trust for %s: %w", commandDir, trustErr)
		}
		return nil
	}

	trusted, lookupErr := IsTrusted(commandDir)
	if lookupErr != nil {
		return fmt.Errorf("check trust for %s: %w", commandDir, lookupErr)
	}
	if trusted {
		return nil
	}

	if !isStdinTTY() {
		return fmt.Errorf("%w: %s — re-run interactively, or set ARARA_TRUST=1 to auto-approve", ErrUntrusted, commandDir)
	}

	return promptForTrust(command, commandDir, options)
}

func promptForTrust(command Command, commandDir string, options ExecOptions) error {
	stderr := orStderrLog(options.Stderr)
	fmt.Fprintf(stderr, "Slash command %q comes from a project directory:\n  %s\nTrust commands from this directory? [y/N/a=always] ", command.Name, commandDir)

	reply := readTrustReply(options.TrustPromptIn)
	switch reply {
	case trustReplyAlways:
		if trustErr := Trust(commandDir); trustErr != nil {
			return fmt.Errorf("persist trust for %s: %w", commandDir, trustErr)
		}
		return nil
	case trustReplyYes:
		return nil
	default:
		return fmt.Errorf("%w: %s — aborted by user", ErrUntrusted, commandDir)
	}
}

func readTrustReply(in io.Reader) string {
	if in == nil {
		in = os.Stdin
	}
	buffer := make([]byte, 8)
	read, _ := in.Read(buffer)
	if read == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(string(buffer[:read])))
}

func orStderrLog(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return os.Stderr
}
