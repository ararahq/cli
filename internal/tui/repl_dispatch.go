package tui

import "strings"

// ActionKind enumerates the discrete operations the REPL recognizes after
// parsing a line of user input. Splitting the kind out from the dispatcher
// keeps the parser pure (no Bubble Tea, no API client) and makes it trivial
// to test.
type ActionKind int

const (
	ActionNoop ActionKind = iota
	ActionQuit
	ActionShowHelp
	ActionClearOutput
	ActionShowWhoami
	ActionShowConfig
	ActionOpenDocs
	ActionShowListenHint
	ActionRunSend
	ActionRunTemplates
	ActionRunStatus
	ActionRunKeys
	ActionRunLogs
	ActionRunCampaigns
	ActionRunContacts
	ActionRunNumbers
	ActionRunEstimate
	ActionRunBalance
	ActionUnknown
)

// Action is the parsed representation of a single REPL input line.
//
//   - Args holds whatever followed the command name (already trimmed).
//   - Command is populated only for ActionUnknown so callers can echo it
//     back to the user in the error path.
//   - RequiresAPIClient indicates whether the action will need an
//     authenticated *api.Client to run. Callers can use this to gate on the
//     login state without hard-coding the same predicate.
type Action struct {
	Kind              ActionKind
	Args              string
	Command           string
	RequiresAPIClient bool
}

// ParseInput tokenizes and classifies a raw REPL line.
//
// It is intentionally pure: same input → same output, no side effects,
// no I/O. The Bubble Tea Update handler is responsible for translating the
// returned Action into actual tea.Cmd values.
func ParseInput(rawInput string) Action {
	trimmed := strings.TrimSpace(rawInput)
	if trimmed == "" {
		return Action{Kind: ActionNoop}
	}

	commandName, commandArgs := splitCommandLine(trimmed)

	// Allow users to type "arara templates" inside the REPL the same way
	// they would from a shell — strip the redundant binary name.
	if commandName == "arara" && commandArgs != "" {
		commandName, commandArgs = splitCommandLine(commandArgs)
	}

	switch commandName {
	case "/quit", "/exit", "/q", "quit", "exit":
		return Action{Kind: ActionQuit}
	case "/help", "help":
		return Action{Kind: ActionShowHelp}
	case "/clear", "clear":
		return Action{Kind: ActionClearOutput}
	case "/whoami", "whoami":
		return Action{Kind: ActionShowWhoami}
	case "/docs", "docs":
		return Action{Kind: ActionOpenDocs}
	case "/config", "config":
		return Action{Kind: ActionShowConfig}
	case "/listen", "listen":
		return Action{Kind: ActionShowListenHint}

	case "/send", "send":
		return Action{Kind: ActionRunSend, Args: commandArgs, RequiresAPIClient: true}
	case "/templates", "templates":
		return Action{Kind: ActionRunTemplates, RequiresAPIClient: true}
	case "/status", "status", "/dash", "dash":
		return Action{Kind: ActionRunStatus, RequiresAPIClient: true}
	case "/keys", "keys":
		return Action{Kind: ActionRunKeys, Args: commandArgs, RequiresAPIClient: true}
	case "/logs", "logs":
		return Action{Kind: ActionRunLogs, RequiresAPIClient: true}
	case "/campaigns", "campaigns":
		return Action{Kind: ActionRunCampaigns, Args: commandArgs, RequiresAPIClient: true}
	case "/contacts", "contacts":
		return Action{Kind: ActionRunContacts, Args: commandArgs, RequiresAPIClient: true}
	case "/numbers", "numbers":
		return Action{Kind: ActionRunNumbers, RequiresAPIClient: true}
	case "/estimate", "estimate":
		return Action{Kind: ActionRunEstimate, Args: commandArgs, RequiresAPIClient: true}
	case "/balance", "balance":
		return Action{Kind: ActionRunBalance, RequiresAPIClient: true}

	default:
		return Action{Kind: ActionUnknown, Command: commandName, Args: commandArgs}
	}
}

// SlashCommandName strips the leading "/" from a slash-style command so the
// caller can use it as a lookup key in the user commands registry. Returns
// the raw name unchanged if there is no leading slash.
func SlashCommandName(commandName string) string {
	if len(commandName) > 1 && commandName[0] == '/' {
		return commandName[1:]
	}
	return commandName
}

// splitCommandLine extracts the (lowercased) command name and the unmodified
// remainder of the line. It is intentionally permissive — it splits only on
// the first run of whitespace and does no quoting.
func splitCommandLine(rawInput string) (string, string) {
	parts := strings.SplitN(rawInput, " ", 2)
	commandName := strings.ToLower(parts[0])

	commandArgs := ""
	if len(parts) > 1 {
		commandArgs = strings.TrimSpace(parts[1])
	}

	return commandName, commandArgs
}
