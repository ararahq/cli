package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

const (
	whoamiLabelWidth = 10
)

type whoamiInfo struct {
	Profile string `json:"profile"`
	Mode    string `json:"mode"`
	APIURL  string `json:"apiUrl"`
	APIKey  string `json:"apiKey"`
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display the current authenticated profile",
	Long:  "Show details about the currently configured profile, including the API mode,\nendpoint URL, and masked API key.",
	RunE:  runWhoami,
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}

func runWhoami(command *cobra.Command, arguments []string) error {
	profileName := GetProfile()

	configuration, loadError := config.Load()
	if loadError != nil {
		output.PrintError(fmt.Sprintf("Failed to load configuration: %s", loadError.Error()))
		return loadError
	}

	activeProfile, profileError := config.GetActiveProfile(configuration)
	if profileError != nil {
		output.PrintError(fmt.Sprintf("No active profile: %s", profileError.Error()))
		return profileError
	}

	apiKey, keyError := config.GetAPIKey(profileName)
	if keyError != nil {
		output.PrintError(fmt.Sprintf("No API key found: %s", keyError.Error()))
		return keyError
	}

	info := buildWhoamiInfo(profileName, activeProfile.APIURL, apiKey)

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(info)
	}

	printWhoamiText(os.Stdout, info)

	return nil
}

func buildWhoamiInfo(profileName string, apiURL string, apiKey string) whoamiInfo {
	client := api.NewClient(apiURL, apiKey)
	return whoamiInfo{
		Profile: profileName,
		Mode:    client.Mode(),
		APIURL:  apiURL,
		APIKey:  maskAPIKey(apiKey),
	}
}

func printWhoamiText(writer io.Writer, info whoamiInfo) {
	fmt.Fprintf(
		writer,
		"%-*s %s\n%-*s %s\n%-*s %s\n%-*s %s\n",
		whoamiLabelWidth, "Profile:", info.Profile,
		whoamiLabelWidth, "Mode:", info.Mode,
		whoamiLabelWidth, "API URL:", info.APIURL,
		whoamiLabelWidth, "API Key:", info.APIKey,
	)
}
