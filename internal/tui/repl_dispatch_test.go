package tui

import (
	"testing"
)

func TestParseInput_BlankAndWhitespace(t *testing.T) {
	cases := []string{"", "   ", "\t", "\n", " \n\t "}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got := ParseInput(input)
			if got.Kind != ActionNoop {
				t.Errorf("ParseInput(%q): want ActionNoop, got %+v", input, got)
			}
		})
	}
}

func TestParseInput_QuitAliases(t *testing.T) {
	for _, input := range []string{"/quit", "/exit", "/q", "quit", "exit", "  QUIT  ", "Exit"} {
		t.Run(input, func(t *testing.T) {
			got := ParseInput(input)
			if got.Kind != ActionQuit {
				t.Errorf("ParseInput(%q): want ActionQuit, got %+v", input, got)
			}
			if got.RequiresAPIClient {
				t.Errorf("quit should not require API client")
			}
		})
	}
}

func TestParseInput_BuiltinCommandsNoArgs(t *testing.T) {
	cases := map[string]ActionKind{
		"/help":   ActionShowHelp,
		"help":    ActionShowHelp,
		"/clear":  ActionClearOutput,
		"clear":   ActionClearOutput,
		"/whoami": ActionShowWhoami,
		"whoami":  ActionShowWhoami,
		"/docs":   ActionOpenDocs,
		"docs":    ActionOpenDocs,
		"/config": ActionShowConfig,
		"config":  ActionShowConfig,
		"/listen": ActionShowListenHint,
		"listen":  ActionShowListenHint,
	}
	for input, wantKind := range cases {
		t.Run(input, func(t *testing.T) {
			got := ParseInput(input)
			if got.Kind != wantKind {
				t.Errorf("ParseInput(%q): want kind %d, got %+v", input, wantKind, got)
			}
			if got.RequiresAPIClient {
				t.Errorf("ParseInput(%q): builtin commands should not require API client", input)
			}
		})
	}
}

func TestParseInput_APICommandsRequireClient(t *testing.T) {
	cases := map[string]ActionKind{
		"/templates": ActionRunTemplates,
		"templates":  ActionRunTemplates,
		"/status":    ActionRunStatus,
		"status":     ActionRunStatus,
		"/dash":      ActionRunStatus,
		"dash":       ActionRunStatus,
		"/logs":      ActionRunLogs,
		"logs":       ActionRunLogs,
		"/numbers":   ActionRunNumbers,
		"numbers":    ActionRunNumbers,
		"/balance":   ActionRunBalance,
		"balance":    ActionRunBalance,
	}
	for input, wantKind := range cases {
		t.Run(input, func(t *testing.T) {
			got := ParseInput(input)
			if got.Kind != wantKind {
				t.Errorf("ParseInput(%q): want kind %d, got %+v", input, wantKind, got)
			}
			if !got.RequiresAPIClient {
				t.Errorf("ParseInput(%q): API commands must require client", input)
			}
		})
	}
}

func TestParseInput_CommandsWithArgs(t *testing.T) {
	cases := []struct {
		input    string
		wantKind ActionKind
		wantArgs string
	}{
		{"send +5511 -t hello", ActionRunSend, "+5511 -t hello"},
		{"/send +5511 hi", ActionRunSend, "+5511 hi"},
		{"keys create test", ActionRunKeys, "create test"},
		{"campaigns camp_123", ActionRunCampaigns, "camp_123"},
		{"contacts search +5511", ActionRunContacts, "search +5511"},
		{"estimate hello 100", ActionRunEstimate, "hello 100"},
		{"  send   +5511 -t welcome  ", ActionRunSend, "+5511 -t welcome"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseInput(tc.input)
			if got.Kind != tc.wantKind {
				t.Errorf("ParseInput(%q): want kind %d, got %+v", tc.input, tc.wantKind, got)
			}
			if got.Args != tc.wantArgs {
				t.Errorf("ParseInput(%q): args want %q, got %q", tc.input, tc.wantArgs, got.Args)
			}
			if !got.RequiresAPIClient {
				t.Errorf("ParseInput(%q): API commands must require client", tc.input)
			}
		})
	}
}

func TestParseInput_StripsAraraPrefix(t *testing.T) {
	cases := []struct {
		input    string
		wantKind ActionKind
		wantArgs string
	}{
		{"arara templates", ActionRunTemplates, ""},
		{"arara send +5511 hi", ActionRunSend, "+5511 hi"},
		{"ARARA help", ActionShowHelp, ""},
		{"  arara   logs  ", ActionRunLogs, ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseInput(tc.input)
			if got.Kind != tc.wantKind {
				t.Errorf("ParseInput(%q): want kind %d, got %+v", tc.input, tc.wantKind, got)
			}
			if got.Args != tc.wantArgs {
				t.Errorf("ParseInput(%q): args want %q, got %q", tc.input, tc.wantArgs, got.Args)
			}
		})
	}
}

func TestParseInput_AraraAlonePassesThrough(t *testing.T) {
	got := ParseInput("arara")
	if got.Kind != ActionUnknown {
		t.Errorf("'arara' alone should be unknown, got %+v", got)
	}
	if got.Command != "arara" {
		t.Errorf("ParseInput preserves unknown command name, got %q", got.Command)
	}
}

func TestParseInput_UnknownCommand(t *testing.T) {
	cases := []struct {
		input       string
		wantCommand string
	}{
		{"banana", "banana"},
		{"/nope", "/nope"},
		{"FROBNICATE all the things", "frobnicate"},
		{"  weird  ", "weird"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseInput(tc.input)
			if got.Kind != ActionUnknown {
				t.Errorf("ParseInput(%q): want ActionUnknown, got %+v", tc.input, got)
			}
			if got.Command != tc.wantCommand {
				t.Errorf("ParseInput(%q): command want %q, got %q", tc.input, tc.wantCommand, got.Command)
			}
		})
	}
}

func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		input       string
		wantCommand string
		wantArgs    string
	}{
		{"foo", "foo", ""},
		{"FOO bar baz", "foo", "bar baz"},
		{"  foo   bar  ", "", "foo   bar"},
		{"send +5511 -t welcome -v João", "send", "+5511 -t welcome -v João"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			gotCommand, gotArgs := splitCommandLine(tc.input)
			if gotCommand != tc.wantCommand {
				t.Errorf("command: want %q, got %q", tc.wantCommand, gotCommand)
			}
			if gotArgs != tc.wantArgs {
				t.Errorf("args: want %q, got %q", tc.wantArgs, gotArgs)
			}
		})
	}
}
