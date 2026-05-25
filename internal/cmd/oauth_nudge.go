package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

// nudgeThrottleFile is the marker file storing the last-shown unix timestamp.
// Lives alongside config.yml in the user's CLI config directory.
const nudgeThrottleFile = ".oauth-nudge-shown-at"

// nudgeIntervalSeconds is the cooldown between nudges (24h). Keeps the
// reminder visible without becoming spammy across day-to-day CLI usage.
const nudgeIntervalSeconds = 86_400

// nudgeSkipCommands are commands where the nudge would be redundant,
// confusing or duplicate. Auth-related commands trigger OAuth on their own
// path; help/version/completion/docs are read-only and orthogonal.
var nudgeSkipCommands = map[string]struct{}{
	"login":      {},
	"logout":     {},
	"help":       {},
	"version":    {},
	"completion": {},
	"docs":       {},
	"manpages":   {},
}

// maybePrintOAuthNudge emits a one-line suggestion to migrate the active
// profile from API key auth to OAuth Device Flow. Silent when:
//   - the active profile already uses OAuth
//   - the command is in nudgeSkipCommands
//   - it has been shown in the last 24h
//   - any read/config error happens (we never block the actual command)
//
// All output goes to stderr so it never pollutes piped stdout.
func maybePrintOAuthNudge(command *cobra.Command) {
	if command == nil {
		return
	}
	if _, skip := nudgeSkipCommands[command.Name()]; skip {
		return
	}
	if shouldSilenceForRoot(command) {
		return
	}

	configuration, loadError := config.Load()
	if loadError != nil {
		return
	}
	profile, profileError := config.GetActiveProfile(configuration)
	if profileError != nil || profile == nil {
		return
	}
	if strings.EqualFold(profile.AuthType, config.AuthTypeOAuth) {
		return
	}

	if !shouldShowNudge() {
		return
	}

	printNudge()
	markNudgeShown()
}

// shouldSilenceForRoot avoids nudging on the bare `arara` invocation that
// drops into the REPL or prints help — those screens already have their own
// onboarding.
func shouldSilenceForRoot(command *cobra.Command) bool {
	return command.Parent() == nil
}

func shouldShowNudge() bool {
	path := nudgeMarkerPath()
	if path == "" {
		return false
	}
	bytes, readError := os.ReadFile(path)
	if readError != nil {
		// File missing or unreadable — show and let markNudgeShown() create it.
		return true
	}
	lastShown, parseError := strconv.ParseInt(strings.TrimSpace(string(bytes)), 10, 64)
	if parseError != nil {
		return true
	}
	return time.Now().Unix()-lastShown >= nudgeIntervalSeconds
}

func markNudgeShown() {
	path := nudgeMarkerPath()
	if path == "" {
		return
	}
	_ = config.EnsureConfigDir()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	// Best-effort: failure here just means we'll nudge again next run.
	_ = os.WriteFile(path, []byte(timestamp), 0o600)
}

func nudgeMarkerPath() string {
	configFile := config.ConfigPath()
	if configFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(configFile), nudgeThrottleFile)
}

func printNudge() {
	dim := output.DimStyle.Render
	bold := output.BoldStyle.Render
	fmt.Fprintln(os.Stderr, dim("─── arara ────────────────────────────────────────────────────"))
	fmt.Fprintf(os.Stderr, "%s Você está autenticado com %s. OAuth Device Flow é o novo padrão recomendado.\n",
		output.WarningStyle.Render("!"), bold("API key"))
	fmt.Fprintf(os.Stderr, "%s Pra migrar: %s\n",
		dim("→"), bold("arara logout && arara login"))
	fmt.Fprintln(os.Stderr, dim("─── (esse aviso aparece 1x por dia) ──────────────────────────"))
}
