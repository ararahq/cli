package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

var modeCmd = &cobra.Command{
	Use:   "mode [live|test]",
	Short: "Switch the active profile between LIVE and TEST",
	Long: `Show the active profile mode, or switch between LIVE and TEST without
copying API keys around.

  arara mode          show the current mode
  arara mode live     create (or reuse) a LIVE key and persist it in the keyring
  arara mode test     drop the LIVE key and fall back to TEST (re-run 'arara login' to refresh the OAuth session)

This command authenticates with whatever credential is already configured
(OAuth session or existing API key) and stores the result in the OS keyring.
The plaintext key is never printed.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMode,
}

func init() {
	rootCmd.AddCommand(modeCmd)
}

func runMode(_ *cobra.Command, arguments []string) error {
	if len(arguments) == 0 {
		return showCurrentMode()
	}

	target := strings.ToLower(strings.TrimSpace(arguments[0]))
	switch target {
	case "live":
		return promoteToLive()
	case "test":
		return demoteToTest()
	default:
		return fmt.Errorf("invalid mode %q -- use 'live' or 'test'", arguments[0])
	}
}

func showCurrentMode() error {
	configuration, loadError := config.Load()
	if loadError != nil {
		return loadError
	}
	profileName := configuration.CurrentProfile
	if profileName == "" {
		profileName = "default"
	}
	profile := configuration.Profiles[profileName]
	mode := strings.ToUpper(profile.Mode)
	if mode == "" {
		mode = strings.ToUpper(config.DefaultMode)
	}
	fmt.Fprintf(os.Stdout, "Profile %s -- %s\n", profileName, mode)
	return nil
}

func promoteToLive() error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}
	client.SetVerbose(IsVerbose())

	if client.IsLiveMode() {
		output.PrintInfo("Already in LIVE mode.")
		return nil
	}

	generatedKey, createError := client.CreateAPIKey("LIVE", resolveKeyName(""))
	if createError != nil {
		output.PrintError(fmt.Sprintf("Failed to create LIVE key: %s", createError.Error()))
		return createError
	}

	profileName := GetProfile()
	if storeError := config.StoreAPIKey(profileName, generatedKey.PlainTextKey); storeError != nil {
		output.PrintError(fmt.Sprintf("Failed to store LIVE key: %s", storeError.Error()))
		return storeError
	}

	if updateError := updateProfileMode(profileName, "live", config.AuthTypeAPIKey); updateError != nil {
		output.PrintError(fmt.Sprintf("Failed to update profile: %s", updateError.Error()))
		return updateError
	}

	output.PrintSuccess(fmt.Sprintf("Switched profile %q to LIVE.", profileName))
	output.PrintInfo("Run 'arara mode test' to revert.")
	return nil
}

func demoteToTest() error {
	profileName := GetProfile()
	configuration, loadError := config.Load()
	if loadError != nil {
		return loadError
	}
	profile := configuration.Profiles[profileName]

	if strings.EqualFold(profile.Mode, "test") {
		output.PrintInfo("Already in TEST mode.")
		return nil
	}

	if deleteError := config.DeleteAPIKey(profileName); deleteError != nil {
		output.PrintError(fmt.Sprintf("Failed to remove LIVE key from keyring: %s", deleteError.Error()))
		return deleteError
	}

	if updateError := updateProfileMode(profileName, "test", config.AuthTypeOAuth); updateError != nil {
		output.PrintError(fmt.Sprintf("Failed to update profile: %s", updateError.Error()))
		return updateError
	}

	output.PrintSuccess(fmt.Sprintf("Switched profile %q to TEST.", profileName))
	output.PrintInfo("Run 'arara login' to refresh the OAuth session before making requests.")
	return nil
}

func updateProfileMode(profileName, mode, authType string) error {
	configuration, loadError := config.Load()
	if loadError != nil {
		return loadError
	}
	profile, exists := configuration.Profiles[profileName]
	if !exists {
		profile = config.Profile{
			APIURL: config.DefaultAPIURL,
			Mode:   config.DefaultMode,
		}
	}
	profile.Mode = mode
	profile.AuthType = authType
	configuration.Profiles[profileName] = profile
	configuration.CurrentProfile = profileName
	return config.Save(configuration)
}
