package cmd

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMCPServer_E2E_DryRunReadOnly exercises the full path from the compiled
// binary through cmd/mcp into internal/mcp, verifying only the read tools are
// registered by default. We use --dry-run to avoid needing credentials or a
// long-running subprocess; protocol-level conformance is covered by
// mark3labs/mcp-go's own test suite and our handler unit tests.
func TestMCPServer_E2E_DryRunReadOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e test builds the binary; skip with -short")
	}

	binaryPath := buildAraraBinary(t)
	tempHome := t.TempDir()

	cmd := exec.Command(binaryPath, "mcp", "--dry-run")
	cmd.Env = append(cmd.Environ(), "HOME="+tempHome)

	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("dry-run failed: %v output=%q", runErr, string(output))
	}

	got := string(output)
	requiredReadTools := []string{
		"arara_list_templates",
		"arara_get_message_status",
		"arara_list_contacts",
		"arara_estimate_campaign",
	}
	for _, want := range requiredReadTools {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in dry-run output, got:\n%s", want, got)
		}
	}

	if strings.Contains(got, "arara_send_message") {
		t.Error("write tools should NOT be exposed by default")
	}
	if !strings.Contains(got, "10 tools registered") {
		t.Errorf("expected 10 tools by default, got:\n%s", got)
	}
}

func TestMCPServer_E2E_DryRunWithWriteTools(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e test builds the binary; skip with -short")
	}

	binaryPath := buildAraraBinary(t)
	tempHome := t.TempDir()

	cmd := exec.Command(binaryPath, "mcp", "--allow-write-tools", "--dry-run")
	cmd.Env = append(cmd.Environ(), "HOME="+tempHome)

	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("dry-run failed: %v output=%q", runErr, string(output))
	}

	got := string(output)
	if !strings.Contains(got, "arara_send_message") {
		t.Errorf("expected arara_send_message in dry-run output with --allow-write-tools, got:\n%s", got)
	}
	if !strings.Contains(got, "arara_create_campaign") {
		t.Errorf("expected arara_create_campaign with --allow-write-tools, got:\n%s", got)
	}
	if !strings.Contains(got, "13 tools registered") {
		t.Errorf("expected 13 tools count with --allow-write-tools, got:\n%s", got)
	}
}

func TestMCPServer_E2E_NoAuthFailsCleanly(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e test builds the binary; skip with -short")
	}

	binaryPath := buildAraraBinary(t)
	tempHome := t.TempDir()

	cmd := exec.Command(binaryPath, "mcp")
	cmd.Env = append(cmd.Environ(), "HOME="+tempHome)

	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("expected non-zero exit when no credentials are configured, got output: %q", string(output))
	}

	if !strings.Contains(string(output), "no API key found") {
		t.Errorf("expected helpful 'no API key found' message, got: %q", string(output))
	}
}

func buildAraraBinary(t *testing.T) string {
	t.Helper()
	tempBinDir := t.TempDir()
	binaryPath := filepath.Join(tempBinDir, "arara")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/arara")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v output=%q", err, string(output))
	}
	return binaryPath
}
