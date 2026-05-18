package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	EventPreSend      = "pre-send"
	EventPostDeliver  = "post-deliver"
	EventPreCampaign  = "pre-campaign"
	EventPostCampaign = "post-campaign"
	EventOnError      = "on-error"

	envEvent   = "ARARA_EVENT"
	envProfile = "ARARA_PROFILE"
	envMode    = "ARARA_MODE"

	defaultTimeout = 30 * time.Second
)

var ErrAborted = errors.New("hook aborted the operation")

type Event struct {
	Name    string
	Profile string
	Mode    string
	Payload any
}

type Outcome struct {
	HookPath string
	ExitCode int
	Stderr   string
	Duration time.Duration
	Err      error
}

type Runner struct {
	Stderr  io.Writer
	Timeout time.Duration

	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
	expandPath  func(string) (string, error)
}

func NewRunner() *Runner {
	return &Runner{
		Stderr:      os.Stderr,
		Timeout:     defaultTimeout,
		execCommand: exec.CommandContext,
		expandPath:  ExpandPath,
	}
}

func (runner *Runner) Run(ctx context.Context, scripts []string, event Event) ([]Outcome, error) {
	if len(scripts) == 0 {
		return nil, nil
	}
	if runner.Timeout <= 0 {
		runner.Timeout = defaultTimeout
	}

	outcomes := make([]Outcome, 0, len(scripts))
	abortable := isAbortableEvent(event.Name)

	payloadJSON, marshalErr := json.Marshal(event.Payload)
	if marshalErr != nil {
		return nil, fmt.Errorf("failed to serialize hook payload: %w", marshalErr)
	}

	for _, script := range scripts {
		outcome := runner.runOne(ctx, script, event, payloadJSON)
		outcomes = append(outcomes, outcome)

		if outcome.Err != nil && abortable {
			return outcomes, fmt.Errorf("%w: %s exited with %d (%w)", ErrAborted, outcome.HookPath, outcome.ExitCode, outcome.Err)
		}

		if outcome.Err != nil {
			fmt.Fprintf(runner.Stderr, "[hook] %s failed (%v) — continuing\n", outcome.HookPath, outcome.Err)
		}
	}

	return outcomes, nil
}

func (runner *Runner) runOne(ctx context.Context, scriptPath string, event Event, payloadJSON []byte) Outcome {
	expandedPath, expandErr := runner.expandPath(scriptPath)
	if expandErr != nil {
		return Outcome{HookPath: scriptPath, Err: fmt.Errorf("failed to expand path: %w", expandErr)}
	}

	if _, statErr := os.Stat(expandedPath); statErr != nil {
		return Outcome{HookPath: expandedPath, Err: fmt.Errorf("hook not found or inaccessible: %w", statErr)}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, runner.Timeout)
	defer cancel()

	command := runner.execCommand(timeoutCtx, expandedPath)
	command.Env = append(os.Environ(),
		envEvent+"="+event.Name,
		envProfile+"="+event.Profile,
		envMode+"="+event.Mode,
	)
	command.Stdin = bytes.NewReader(payloadJSON)

	stderrBuffer := &bytes.Buffer{}
	command.Stderr = stderrBuffer
	command.Stdout = io.Discard

	startedAt := time.Now()
	runErr := command.Run()
	duration := time.Since(startedAt)

	outcome := Outcome{
		HookPath: expandedPath,
		Stderr:   stderrBuffer.String(),
		Duration: duration,
	}

	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		outcome.Err = fmt.Errorf("hook timed out after %s", runner.Timeout)
		return outcome
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			outcome.ExitCode = exitErr.ExitCode()
		}
		outcome.Err = runErr
		return outcome
	}

	return outcome
}

func ExpandPath(path string) (string, error) {
	expanded := strings.TrimSpace(path)
	if expanded == "" {
		return "", errors.New("hook path is empty")
	}

	if strings.HasPrefix(expanded, "~/") || expanded == "~" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", homeErr
		}
		expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
	}

	if !filepath.IsAbs(expanded) {
		absolute, absErr := filepath.Abs(expanded)
		if absErr != nil {
			return "", absErr
		}
		return absolute, nil
	}
	return expanded, nil
}

func isAbortableEvent(eventName string) bool {
	switch eventName {
	case EventPreSend, EventPreCampaign:
		return true
	default:
		return false
	}
}
