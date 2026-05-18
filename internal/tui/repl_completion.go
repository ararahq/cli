package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// completionSuggestions is the list bubbles/textinput uses for inline tab
// completion in the REPL. We include both the bare and the /-prefixed
// variant of each command so typing `te<tab>` completes to `templates`
// and typing `/h<tab>` completes to `/help`.
//
// Order matters: bubbles picks the first prefix match, so the most-used
// commands come first.
var completionSuggestions = []string{
	// Top of mind for new users.
	"templates",
	"send ",
	"status",
	"balance",
	"help",
	// Operational / read-only.
	"campaigns",
	"contacts",
	"numbers",
	"keys",
	"logs",
	"estimate",
	"whoami",
	"config",
	"docs",
	"listen",
	"clear",
	"quit",
	"exit",
	// Slash variants — same set with leading slash for users who came in
	// from the Claude Code / Stripe CLI muscle memory of typing `/`.
	"/templates",
	"/send ",
	"/status",
	"/balance",
	"/help",
	"/campaigns",
	"/contacts",
	"/numbers",
	"/keys",
	"/logs",
	"/estimate",
	"/whoami",
	"/config",
	"/docs",
	"/listen",
	"/clear",
	"/quit",
}

// placeholderRotation is the cycle of example commands shown in the empty
// input field. It rotates every placeholderInterval so the prompt feels
// alive and discoverable instead of frozen on a single hint.
var placeholderRotation = []string{
	"templates",
	"send +5511999999999 hello!",
	"status",
	"campaigns list",
	"balance",
	"estimate hello_world 500",
	"/help",
}

const placeholderInterval = 3 * time.Second

// placeholderTickMsg fires on a fixed interval to advance the placeholder
// rotation. Defining a typed message instead of reusing tea.Tick's raw
// time.Time keeps the Update switch readable.
type placeholderTickMsg time.Time

func placeholderTickCmd() tea.Cmd {
	return tea.Tick(placeholderInterval, func(t time.Time) tea.Msg {
		return placeholderTickMsg(t)
	})
}

// inputHintFor returns the canonical command that the typed prefix would
// dispatch to, or empty string when there's no clear match yet. We use it
// to render a subtle "↳ runs <command>" hint below the prompt so the user
// sees what their next Enter will do *before* they commit.
//
// Distinct from the textinput suggestions (which are visual greyed-out
// completions) — this is an additional read-out that survives even when
// the cursor isn't at the end of the line.
func inputHintFor(rawInput string) string {
	trimmed := strings.TrimSpace(rawInput)
	if trimmed == "" {
		return ""
	}

	// Don't hint on the canonical name itself — only on partial matches.
	commandName, _ := splitCommandLine(trimmed)
	for _, suggestion := range completionSuggestions {
		canonical := strings.TrimSpace(suggestion)
		if canonical == commandName {
			return ""
		}
		if strings.HasPrefix(canonical, commandName) && canonical != commandName {
			return canonical
		}
	}
	return ""
}
