package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

const (
	configKeyProfile = "profile"
	configKeyAPIURL  = "api-url"
	configKeyOutput  = "output"
	configKeyMode    = "mode"

	configLabelWidth = 16
)

var validConfigKeys = map[string]bool{
	configKeyProfile: true,
	configKeyAPIURL:  true,
	configKeyOutput:  true,
	configKeyMode:    true,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  "View and modify CLI configuration settings such as the active profile,\nAPI URL, output format, and operating mode.",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all configuration values",
	RunE:  runConfigList,
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Long:  "Set a configuration value. Valid keys: profile, api-url, output, mode",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

func init() {
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)

	rootCmd.AddCommand(configCmd)
}

func runConfigList(command *cobra.Command, arguments []string) error {
	configuration, loadError := config.Load()
	if loadError != nil {
		output.PrintError(fmt.Sprintf("Failed to load configuration: %s", loadError.Error()))
		return loadError
	}

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(configuration)
	}

	printConfigValues(configuration)

	return nil
}

func runConfigSet(command *cobra.Command, arguments []string) error {
	key := arguments[0]
	value := arguments[1]

	if !validConfigKeys[key] {
		validationError := fmt.Errorf("invalid config key %q -- valid keys: profile, api-url, output, mode", key)
		output.PrintError(validationError.Error())
		return validationError
	}

	configuration, loadError := config.Load()
	if loadError != nil {
		output.PrintError(fmt.Sprintf("Failed to load configuration: %s", loadError.Error()))
		return loadError
	}

	applyConfigValue(configuration, key, value)

	if saveError := config.Save(configuration); saveError != nil {
		output.PrintError(fmt.Sprintf("Failed to save configuration: %s", saveError.Error()))
		return saveError
	}

	output.PrintSuccess(fmt.Sprintf("Set %s = %s", key, value))

	return nil
}

func runConfigGet(command *cobra.Command, arguments []string) error {
	key := arguments[0]

	if !validConfigKeys[key] {
		validationError := fmt.Errorf("invalid config key %q -- valid keys: profile, api-url, output, mode", key)
		output.PrintError(validationError.Error())
		return validationError
	}

	configuration, loadError := config.Load()
	if loadError != nil {
		output.PrintError(fmt.Sprintf("Failed to load configuration: %s", loadError.Error()))
		return loadError
	}

	value := resolveConfigValue(configuration, key)
	fmt.Fprintln(os.Stdout, value)

	return nil
}

func applyConfigValue(configuration *config.Config, key string, value string) {
	profileName := configuration.CurrentProfile
	if profileName == "" {
		profileName = "default"
	}

	profile, exists := configuration.Profiles[profileName]
	if !exists {
		profile = config.Profile{
			APIURL: config.DefaultAPIURL,
			Mode:   config.DefaultMode,
		}
	}

	switch key {
	case configKeyProfile:
		configuration.CurrentProfile = value
	case configKeyAPIURL:
		profile.APIURL = value
		configuration.Profiles[profileName] = profile
	case configKeyOutput:
		configuration.Output = value
	case configKeyMode:
		profile.Mode = value
		configuration.Profiles[profileName] = profile
	}
}

func resolveConfigValue(configuration *config.Config, key string) string {
	profileName := configuration.CurrentProfile
	if profileName == "" {
		profileName = "default"
	}

	profile := configuration.Profiles[profileName]

	switch key {
	case configKeyProfile:
		return configuration.CurrentProfile
	case configKeyAPIURL:
		if profile.APIURL != "" {
			return profile.APIURL
		}
		return config.DefaultAPIURL
	case configKeyOutput:
		if configuration.Output != "" {
			return configuration.Output
		}
		return config.DefaultOutput
	case configKeyMode:
		if profile.Mode != "" {
			return profile.Mode
		}
		return config.DefaultMode
	default:
		return ""
	}
}

func printConfigValues(configuration *config.Config) {
	profileName := configuration.CurrentProfile
	if profileName == "" {
		profileName = "(none)"
	}

	fmt.Fprintf(os.Stdout, "  %-*s %s\n", configLabelWidth, "Profile:", profileName)
	fmt.Fprintf(os.Stdout, "  %-*s %s\n", configLabelWidth, "Output:", configuration.Output)

	profile := configuration.Profiles[configuration.CurrentProfile]
	apiURL := profile.APIURL
	if apiURL == "" {
		apiURL = config.DefaultAPIURL
	}

	mode := profile.Mode
	if mode == "" {
		mode = config.DefaultMode
	}

	fmt.Fprintf(os.Stdout, "  %-*s %s\n", configLabelWidth, "API URL:", apiURL)
	fmt.Fprintf(os.Stdout, "  %-*s %s\n", configLabelWidth, "Mode:", mode)
	fmt.Fprintf(os.Stdout, "  %-*s %s\n", configLabelWidth, "Config Path:", config.ConfigPath())
}
