package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/telemetry"
)

// telemetryRecorderOnce builds (and lazily caches) the process-wide Recorder
// used by all command wrappers via PersistentPreRun/PostRun. We resolve
// state from disk once, then every Cobra hook reads from the same instance.
var (
	telemetryRecorderOnce sync.Once
	telemetryRecorderInst *telemetry.Recorder
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage anonymous CLI usage telemetry (off by default)",
	Long: "Telemetry sends a small set of anonymized metrics — command name, exit\n" +
		"code, latency, CLI version, platform, and a locally-generated UUID — to\n" +
		"AraraHQ so we can prioritize improvements based on real usage. We never\n" +
		"capture phone numbers, message bodies, recipient names, template content,\n" +
		"or API keys. Off by default; opt in with `arara telemetry on`.",
}

var telemetryOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Enable anonymous telemetry",
	RunE:  runTelemetryOn,
}

var telemetryOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Disable telemetry and stop sending events",
	RunE:  runTelemetryOff,
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether telemetry is enabled and the local anonymous id",
	RunE:  runTelemetryStatus,
}

func init() {
	telemetryCmd.AddCommand(telemetryOnCmd)
	telemetryCmd.AddCommand(telemetryOffCmd)
	telemetryCmd.AddCommand(telemetryStatusCmd)
	rootCmd.AddCommand(telemetryCmd)
}

func runTelemetryOn(_ *cobra.Command, _ []string) error {
	home, err := userHomeDir()
	if err != nil {
		return err
	}
	state, loadErr := telemetry.LoadState(home)
	if loadErr != nil {
		return loadErr
	}
	state.Enabled = true
	state, ensureErr := telemetry.EnsureAnonymousID(home, state)
	if ensureErr != nil {
		return ensureErr
	}
	if saveErr := telemetry.SaveState(home, state); saveErr != nil {
		return saveErr
	}

	output.PrintSuccess("Telemetry enabled.")
	output.PrintInfo(fmt.Sprintf("Anonymous ID: %s", state.AnonymousID))
	output.PrintInfo("We collect: command name, exit code, latency, CLI version, platform.")
	output.PrintInfo("We never collect: phone numbers, message bodies, contacts, template content, API keys.")
	return nil
}

func runTelemetryOff(_ *cobra.Command, _ []string) error {
	home, err := userHomeDir()
	if err != nil {
		return err
	}
	state, loadErr := telemetry.LoadState(home)
	if loadErr != nil {
		return loadErr
	}
	state.Enabled = false
	if saveErr := telemetry.SaveState(home, state); saveErr != nil {
		return saveErr
	}

	output.PrintSuccess("Telemetry disabled. No further events will be sent.")
	return nil
}

func runTelemetryStatus(_ *cobra.Command, _ []string) error {
	home, err := userHomeDir()
	if err != nil {
		return err
	}
	state, loadErr := telemetry.LoadState(home)
	if loadErr != nil {
		return loadErr
	}

	return printTelemetryStatus(os.Stdout, state)
}

func printTelemetryStatus(writer io.Writer, state telemetry.State) error {
	if state.Enabled {
		fmt.Fprintln(writer, output.SuccessStyle.Render("enabled"))
	} else {
		fmt.Fprintln(writer, output.DimStyle.Render("disabled"))
	}
	if state.AnonymousID != "" {
		fmt.Fprintf(writer, "anonymous id: %s\n", state.AnonymousID)
	}
	return nil
}

// resolveTelemetryRecorder returns the singleton Recorder used by Cobra
// hooks. Built lazily so commands like `arara --help` don't pay the cost
// of disk I/O.
func resolveTelemetryRecorder() *telemetry.Recorder {
	telemetryRecorderOnce.Do(func() {
		home, err := userHomeDir()
		if err != nil {
			return
		}
		state, loadErr := telemetry.LoadState(home)
		if loadErr != nil || !state.Enabled || state.AnonymousID == "" {
			return
		}
		baseURL := defaultTelemetryBaseURL()
		if baseURL == "" {
			return
		}
		telemetryRecorderInst = telemetry.NewRecorder(state, telemetry.NewHTTPSender(baseURL))
	})
	return telemetryRecorderInst
}

// defaultTelemetryBaseURL pulls the API URL from the active config so
// telemetry follows the same environment (staging/prod) the user is on.
func defaultTelemetryBaseURL() string {
	client, err := newClientForCmd()
	if err != nil || client == nil {
		return ""
	}
	return client.BaseURL()
}

func userHomeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return dir, nil
}

// silence unused-import warning when api isn't directly referenced.
var _ = api.NewClient
