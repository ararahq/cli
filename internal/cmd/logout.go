package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials for the current profile",
	Long:  "Remove the API key from the OS keyring and delete the profile from the config file.\nThis does not revoke the key on the server — use 'arara keys revoke' for that.",
	RunE:  runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(command *cobra.Command, arguments []string) error {
	profileName := GetProfile()

	if deleteKeyError := config.DeleteAPIKey(profileName); deleteKeyError != nil {
		output.PrintError(fmt.Sprintf("Failed to remove API key from keyring: %s", deleteKeyError.Error()))
		return deleteKeyError
	}

	if removeError := removeProfileFromConfig(profileName); removeError != nil {
		output.PrintError(fmt.Sprintf("Failed to remove profile from config: %s", removeError.Error()))
		return removeError
	}

	output.PrintSuccess(fmt.Sprintf("Logged out from profile %q. Credentials removed.", profileName))

	return nil
}

func removeProfileFromConfig(profileName string) error {
	configuration, loadError := config.Load()
	if loadError != nil {
		return fmt.Errorf("failed to load configuration: %w", loadError)
	}

	delete(configuration.Profiles, profileName)

	if configuration.CurrentProfile == profileName {
		configuration.CurrentProfile = selectRemainingProfile(configuration)
	}

	if saveError := config.Save(configuration); saveError != nil {
		return fmt.Errorf("failed to save configuration after profile removal: %w", saveError)
	}

	return nil
}

func selectRemainingProfile(configuration *config.Config) string {
	for remainingName := range configuration.Profiles {
		return remainingName
	}

	return ""
}
