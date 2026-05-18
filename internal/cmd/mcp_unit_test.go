package cmd

import (
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/api"
)

func TestResolveMCPAPIClient_DryRunUsesPlaceholder(t *testing.T) {
	client, err := resolveMCPAPIClient(true)
	if err != nil {
		t.Fatalf("resolveMCPAPIClient(true): %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if !strings.HasPrefix(client.BaseURL(), api.DefaultMCPDryRunURL) {
		t.Errorf("expected dry-run base URL, got %q", client.BaseURL())
	}
}

func TestResolveMCPAPIClient_NonDryRunUsesInjected(t *testing.T) {
	_, fakeClient := fakeAPIServerJSON(t, 200, `{}`)
	withFakeAPIClient(t, fakeClient)

	got, err := resolveMCPAPIClient(false)
	if err != nil {
		t.Fatalf("resolveMCPAPIClient(false): %v", err)
	}
	if got != fakeClient {
		t.Errorf("expected injected client, got %v", got)
	}
}

func TestResolveAllowWriteTools_FlagWins(t *testing.T) {
	original := mcpAllowWriteToolsFlag
	t.Cleanup(func() { mcpAllowWriteToolsFlag = original })

	mcpAllowWriteToolsFlag = true
	if !resolveAllowWriteTools() {
		t.Error("flag=true should resolve to true")
	}

	mcpAllowWriteToolsFlag = false
	// Without the flag we end up reading settings, which in a fresh tempdir
	// HOME defaults to false. We use t.Setenv to ensure a clean state.
	t.Setenv("HOME", t.TempDir())
	if resolveAllowWriteTools() {
		t.Error("flag=false + no settings should resolve to false")
	}
}

func TestPrintRegisteredTools_SmokesOutput(t *testing.T) {
	// Just exercise the path — output goes to stderr.
	printRegisteredTools([]string{"arara_list_templates", "arara_send_message"}, true)
	printRegisteredTools(nil, false)
}

func TestRunMCPServer_DryRunListsToolsAndExits(t *testing.T) {
	originalDry := mcpDryRunFlag
	originalAllow := mcpAllowWriteToolsFlag
	t.Cleanup(func() {
		mcpDryRunFlag = originalDry
		mcpAllowWriteToolsFlag = originalAllow
	})

	mcpDryRunFlag = true
	mcpAllowWriteToolsFlag = false

	if err := runMCPServer(nil, nil); err != nil {
		t.Errorf("runMCPServer --dry-run: %v", err)
	}
}
