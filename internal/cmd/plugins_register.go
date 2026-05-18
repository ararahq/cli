package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/plugins"
	"github.com/ararahq/cli/internal/settings"
)

const (
	pluginEnvProfile     = "ARARA_PROFILE"
	pluginEnvOutput      = "ARARA_OUTPUT"
	pluginEnvMode        = "ARARA_MODE"
	pluginEnvVerbose     = "ARARA_VERBOSE"
	pluginCommandGroupID = "plugins"
)

func registerDiscoveredPlugins(rootCommand *cobra.Command) {
	resolved, _ := settings.Load("")

	var extraDirs []string
	var disabled []string
	if resolved != nil {
		for _, dir := range resolved.Settings.Plugins.SearchPaths {
			expanded, err := plugins.ExpandPath(dir)
			if err == nil {
				extraDirs = append(extraDirs, expanded)
			}
		}
		disabled = resolved.Settings.Plugins.Disabled
	}

	discoverer := plugins.NewDiscoverer(os.Getenv("PATH"), extraDirs, disabled)
	discovered, err := discoverer.Discover()
	if err != nil || len(discovered) == 0 {
		return
	}

	registerPluginCommands(rootCommand, discovered)
}

func registerPluginCommands(rootCommand *cobra.Command, discovered []plugins.Plugin) {
	rootCommand.AddGroup(&cobra.Group{
		ID:    pluginCommandGroupID,
		Title: "Plugins:",
	})

	for _, plugin := range discovered {
		// commandAlreadyRegistered does the actual lookup. The leftover
		// rootCommand.Find call here was dead — it returned a *Command
		// and an error neither of which we used. Removed in favor of the
		// authoritative check below.
		if commandAlreadyRegistered(rootCommand, plugin.Name) {
			continue
		}

		pluginCopy := plugin
		shortDescription := plugin.Description
		if shortDescription == "" {
			shortDescription = "external plugin"
		}

		pluginCommand := &cobra.Command{
			Use:                pluginCopy.Name,
			Short:              shortDescription,
			GroupID:            pluginCommandGroupID,
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, arguments []string) error {
				executor := plugins.NewExecutor()
				executor.GlobalEnv = buildPluginEnv()
				return executor.Execute(context.Background(), pluginCopy, arguments)
			},
		}

		rootCommand.AddCommand(pluginCommand)
	}
}

func commandAlreadyRegistered(rootCommand *cobra.Command, name string) bool {
	for _, sub := range rootCommand.Commands() {
		if sub.Name() == name {
			return true
		}
	}
	return false
}

func buildPluginEnv() map[string]string {
	env := map[string]string{}
	if globalProfile != "" {
		env[pluginEnvProfile] = globalProfile
	}
	if globalOutputFormat != "" {
		env[pluginEnvOutput] = globalOutputFormat
	}
	if globalMode != "" {
		env[pluginEnvMode] = globalMode
	}
	if globalVerbose {
		env[pluginEnvVerbose] = "1"
	}
	return env
}
