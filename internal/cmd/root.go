package cmd

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/telemetry"
	"github.com/ararahq/cli/internal/tui"
	"github.com/ararahq/cli/internal/version"
)

const (
	defaultOutputFormat = "text"
	defaultProfile      = "default"
)

var (
	globalOutputFormat string
	globalProfile      string
	globalMode         string
	globalTheme        string
	globalJQ           string
	globalVerbose      bool
	globalNoColor      bool
)

// commandStartTimes holds the wall-clock at which the current command's
// PersistentPreRun fired, so PersistentPostRunE can compute durationMs
// for telemetry. Keyed by *cobra.Command pointer in case sibling commands
// somehow run concurrently inside a single process (REPL).
var commandStartTimes sync.Map

var rootCmd = &cobra.Command{
	Use:           "arara",
	Short:         "AraraHQ CLI \u2014 WhatsApp API Platform",
	Long:          "AraraHQ CLI is the command-line interface for the AraraHQ Revenue OS.\nManage WhatsApp templates, send messages, configure API keys,\nand interact with the full WhatsApp Business API from your terminal.",
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(command *cobra.Command, arguments []string) {
		applyGlobalFlags()
		commandStartTimes.Store(command, time.Now())
	},
	PersistentPostRunE: func(command *cobra.Command, arguments []string) error {
		recordCommandTelemetry(command, 0)
		return nil
	},
	RunE: func(command *cobra.Command, arguments []string) error {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return tui.RunREPL()
		}

		output.PrintHeader()
		fmt.Println()
		return command.Help()
	},
}

// recordCommandTelemetry enqueues an Event for the just-completed command
// and flushes synchronously (with a short timeout) so the process can exit
// without leaking events. exitCode=0 from PostRun; the wrapper layer in
// Execute() rewrites it on error before flush.
func recordCommandTelemetry(command *cobra.Command, exitCode int) {
	recorder := resolveTelemetryRecorder()
	if !recorder.Enabled() {
		return
	}

	startedAtRaw, ok := commandStartTimes.LoadAndDelete(command)
	if !ok {
		return
	}
	startedAt, ok := startedAtRaw.(time.Time)
	if !ok {
		return
	}

	recorder.Record(telemetry.Event{
		Command:    command.Name(),
		ExitCode:   exitCode,
		DurationMs: int(time.Since(startedAt).Milliseconds()),
		CLIVersion: version.Version,
	})
}

const helpTemplate = `{{with .Long}}{{. | trimTrailingWhitespaces}}{{end}}

{{if .HasAvailableSubCommands}}Commands:{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}

{{if .HasAvailableLocalFlags}}Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

{{if and .HasAvailableInheritedFlags (not .HasParent)}}Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{else if .HasAvailableInheritedFlags}}Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

Use "{{.CommandPath}} [command] --help" for more information about a command.
`

func init() {
	rootCmd.SetHelpTemplate(helpTemplate)

	rootCmd.PersistentFlags().StringVarP(&globalOutputFormat, "output", "o", defaultOutputFormat, "output format: text, json, table, stream-json")
	rootCmd.PersistentFlags().StringVarP(&globalProfile, "profile", "p", defaultProfile, "configuration profile name")
	rootCmd.PersistentFlags().StringVarP(&globalMode, "mode", "m", "", "operating mode (sandbox or production)")
	rootCmd.PersistentFlags().StringVar(&globalTheme, "theme", "", "color theme: auto (default), mono. Overrides ARARA_THEME and NO_COLOR.")
	rootCmd.PersistentFlags().StringVar(&globalJQ, "jq", "", "filter JSON output through a jq expression (e.g. '.[] | .name')")
	rootCmd.PersistentFlags().BoolVarP(&globalVerbose, "verbose", "v", false, "enable verbose output for debugging")
	rootCmd.PersistentFlags().BoolVar(&globalNoColor, "no-color", false, "disable colored output (alias for --theme=mono)")
}

func Execute() error {
	registerDiscoveredPlugins(rootCmd)
	registerDiscoveredCommands(rootCmd)
	executeError := rootCmd.Execute()
	flushTelemetryQuietly()
	return executeError
}

// flushTelemetryQuietly drains the recorder on process exit. Errors are
// swallowed (printed only in verbose mode) — telemetry never blocks exit.
func flushTelemetryQuietly() {
	recorder := resolveTelemetryRecorder()
	if !recorder.Enabled() {
		return
	}
	if flushErr := recorder.Flush(); flushErr != nil && IsVerbose() {
		fmt.Fprintf(os.Stderr, "[telemetry] flush: %v\n", flushErr)
	}
}

func applyGlobalFlags() {
	if globalNoColor {
		os.Setenv("NO_COLOR", "1")
		output.ApplyTheme(output.ThemeMono)
		return
	}

	if globalTheme != "" {
		output.ApplyTheme(output.ParseThemeName(globalTheme))
		return
	}

	if envTheme := os.Getenv("ARARA_THEME"); envTheme != "" {
		output.ApplyTheme(output.ParseThemeName(envTheme))
		return
	}

	if os.Getenv("NO_COLOR") != "" {
		output.ApplyTheme(output.ThemeMono)
	}
}

func GetOutputFormat() output.Format {
	return output.ParseFormat(globalOutputFormat)
}

// GetJQ returns the jq expression configured via --jq, or empty string when
// the user did not opt in. Commands that produce JSON output should pass
// this through ApplyJQ before rendering so the filter is honored uniformly.
func GetJQ() string {
	return globalJQ
}

func GetProfile() string {
	return globalProfile
}

func GetMode() string {
	return globalMode
}

func IsVerbose() bool {
	return globalVerbose
}
