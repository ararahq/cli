package cmd

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/plugins"
)

func TestBuildPluginEnv_OmitsEmptyValues(t *testing.T) {
	originalProfile := globalProfile
	originalOutput := globalOutputFormat
	originalMode := globalMode
	originalVerbose := globalVerbose
	t.Cleanup(func() {
		globalProfile = originalProfile
		globalOutputFormat = originalOutput
		globalMode = originalMode
		globalVerbose = originalVerbose
	})

	globalProfile = ""
	globalOutputFormat = ""
	globalMode = ""
	globalVerbose = false

	env := buildPluginEnv()
	if len(env) != 0 {
		t.Errorf("empty globals should produce empty env, got %v", env)
	}
}

func TestBuildPluginEnv_PopulatesSetValues(t *testing.T) {
	originalProfile := globalProfile
	originalOutput := globalOutputFormat
	originalMode := globalMode
	originalVerbose := globalVerbose
	t.Cleanup(func() {
		globalProfile = originalProfile
		globalOutputFormat = originalOutput
		globalMode = originalMode
		globalVerbose = originalVerbose
	})

	globalProfile = "prod"
	globalOutputFormat = "json"
	globalMode = "live"
	globalVerbose = true

	env := buildPluginEnv()
	expected := map[string]string{
		pluginEnvProfile: "prod",
		pluginEnvOutput:  "json",
		pluginEnvMode:    "live",
		pluginEnvVerbose: "1",
	}
	for key, want := range expected {
		if env[key] != want {
			t.Errorf("env[%q]: want %q, got %q", key, want, env[key])
		}
	}
}

func TestCommandAlreadyRegistered_DetectsExisting(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "send"})

	if !commandAlreadyRegistered(root, "send") {
		t.Error("send should be detected as already registered")
	}
	if commandAlreadyRegistered(root, "newcmd") {
		t.Error("newcmd should not be detected as registered")
	}
}

func TestRegisterPluginCommands_RegistersDiscovered(t *testing.T) {
	root := &cobra.Command{Use: "root"}

	registerPluginCommands(root, []plugins.Plugin{
		{Name: "extra-cmd", Description: "external plugin"},
	})

	if !commandAlreadyRegistered(root, "extra-cmd") {
		t.Error("plugin command should be added to root")
	}
}

func TestRegisterPluginCommands_SkipsConflicts(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "send"})

	registerPluginCommands(root, []plugins.Plugin{
		{Name: "send", Description: "would shadow builtin"},
	})

	// Only one command named send should remain (the original).
	count := 0
	for _, child := range root.Commands() {
		if child.Name() == "send" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 send, got %d", count)
	}
}
