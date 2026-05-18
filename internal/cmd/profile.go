package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

const profileLabelWidth = 14

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage CLI profiles (multi-org / multi-team)",
	Long: "List, switch, add, and remove configured profiles. Each profile holds\n" +
		"its own API key, API URL, and mode. Useful when you operate multiple\n" +
		"AraraHQ orgs (e.g., agency managing several clients) — switch with\n" +
		"`arara profile switch <name>` instead of editing config by hand.",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles",
	RunE:  runProfileList,
}

var profileSwitchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Set the active profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileSwitch,
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Register a new profile (use 'arara login --profile <name>' afterwards)",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileAdd,
}

var profileRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a profile and its stored credentials",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileRemove,
}

func init() {
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileSwitchCmd)
	profileCmd.AddCommand(profileAddCmd)
	profileCmd.AddCommand(profileRemoveCmd)

	rootCmd.AddCommand(profileCmd)
}

// profileSummary is the row shape for both text and JSON output. We avoid
// inlining the full Profile so we never accidentally surface API keys here.
type profileSummary struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	APIURL string `json:"apiUrl"`
	Mode   string `json:"mode"`
	HasKey bool   `json:"hasKey"`
}

func runProfileList(_ *cobra.Command, _ []string) error {
	configuration, loadErr := config.Load()
	if loadErr != nil {
		output.PrintError(fmt.Sprintf("Failed to load config: %s", loadErr.Error()))
		return loadErr
	}

	summaries := buildProfileSummaries(configuration)

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(summaries)
	}
	if GetOutputFormat() == output.FormatStreamJSON {
		for _, summary := range summaries {
			if writeErr := output.PrintJSONLine(summary); writeErr != nil {
				return writeErr
			}
		}
		return nil
	}

	printProfileSummaries(os.Stdout, summaries)
	return nil
}

func runProfileSwitch(_ *cobra.Command, arguments []string) error {
	targetName := arguments[0]

	configuration, loadErr := config.Load()
	if loadErr != nil {
		return loadErr
	}

	if _, exists := configuration.Profiles[targetName]; !exists {
		err := fmt.Errorf("profile %q not found — known profiles: %s", targetName, profileNamesForError(configuration))
		output.PrintError(err.Error())
		return err
	}

	configuration.CurrentProfile = targetName
	if saveErr := config.Save(configuration); saveErr != nil {
		output.PrintError(fmt.Sprintf("Failed to save config: %s", saveErr.Error()))
		return saveErr
	}

	output.PrintSuccess(fmt.Sprintf("Active profile is now %q", targetName))
	return nil
}

func runProfileAdd(_ *cobra.Command, arguments []string) error {
	newName := arguments[0]

	configuration, loadErr := config.Load()
	if loadErr != nil {
		return loadErr
	}

	if _, exists := configuration.Profiles[newName]; exists {
		err := fmt.Errorf("profile %q already exists — use 'arara profile switch %s'", newName, newName)
		output.PrintError(err.Error())
		return err
	}

	configuration.Profiles[newName] = config.Profile{
		APIURL: config.DefaultAPIURL,
		Mode:   config.DefaultMode,
	}
	if saveErr := config.Save(configuration); saveErr != nil {
		return saveErr
	}

	output.PrintSuccess(fmt.Sprintf("Profile %q created. Run 'arara login --profile %s' to add credentials.", newName, newName))
	return nil
}

func runProfileRemove(_ *cobra.Command, arguments []string) error {
	target := arguments[0]

	configuration, loadErr := config.Load()
	if loadErr != nil {
		return loadErr
	}

	if _, exists := configuration.Profiles[target]; !exists {
		err := fmt.Errorf("profile %q not found", target)
		output.PrintError(err.Error())
		return err
	}

	if deleteErr := config.DeleteAPIKey(target); deleteErr != nil {
		// Not fatal — profile may not have stored a key. We surface a warning
		// so the user knows credentials may still be in the keyring under a
		// stale name, but proceed with profile removal anyway.
		output.PrintWarning(fmt.Sprintf("Could not remove keyring entry for %q: %s", target, deleteErr.Error()))
	}

	delete(configuration.Profiles, target)

	if configuration.CurrentProfile == target {
		configuration.CurrentProfile = pickAnyProfile(configuration)
	}

	if saveErr := config.Save(configuration); saveErr != nil {
		return saveErr
	}

	output.PrintSuccess(fmt.Sprintf("Profile %q removed.", target))
	if configuration.CurrentProfile == "" {
		output.PrintWarning("No profiles remain — run 'arara login' to create one.")
	} else if configuration.CurrentProfile != target {
		output.PrintInfo(fmt.Sprintf("Active profile is now %q", configuration.CurrentProfile))
	}
	return nil
}

func buildProfileSummaries(configuration *config.Config) []profileSummary {
	summaries := make([]profileSummary, 0, len(configuration.Profiles))
	for name, profile := range configuration.Profiles {
		summaries = append(summaries, profileSummary{
			Name:   name,
			Active: name == configuration.CurrentProfile,
			APIURL: nonEmpty(profile.APIURL, config.DefaultAPIURL),
			Mode:   nonEmpty(profile.Mode, config.DefaultMode),
			HasKey: profileHasStoredKey(name, profile),
		})
	}

	sort.Slice(summaries, func(left, right int) bool {
		return summaries[left].Name < summaries[right].Name
	})
	return summaries
}

// profileHasStoredKey reports whether the profile has credentials available
// — either inline in the config (legacy fallback) or in the OS keyring.
// Never surfaces the key value itself.
func profileHasStoredKey(name string, profile config.Profile) bool {
	if profile.APIKey != "" {
		return true
	}
	if storedKey, err := config.GetAPIKey(name); err == nil && storedKey != "" {
		return true
	}
	return false
}

func printProfileSummaries(writer io.Writer, summaries []profileSummary) {
	if len(summaries) == 0 {
		output.PrintInfo("No profiles configured. Run 'arara login' to create one.")
		return
	}

	for _, summary := range summaries {
		marker := " "
		if summary.Active {
			marker = output.SuccessStyle.Render("*")
		}

		nameRendered := summary.Name
		if summary.Active {
			nameRendered = output.BoldStyle.Render(summary.Name)
		}

		keyMark := output.DimStyle.Render("(no credentials)")
		if summary.HasKey {
			keyMark = output.SuccessStyle.Render("✓")
		}

		fmt.Fprintf(writer, "%s %-*s %s  %s  %s\n",
			marker,
			profileLabelWidth, nameRendered,
			output.DimStyle.Render(summary.Mode),
			summary.APIURL,
			keyMark,
		)
	}
}

func pickAnyProfile(configuration *config.Config) string {
	for name := range configuration.Profiles {
		return name
	}
	return ""
}

func profileNamesForError(configuration *config.Config) string {
	names := make([]string, 0, len(configuration.Profiles))
	for name := range configuration.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// `api` import is consumed by other files in this package; the explicit
// blank reference here keeps profile.go importing the package even when
// goimports tries to clean it up. (Until profile commands themselves call
// the API directly, we leave the indirect dependency explicit.)
var _ = api.NewClient
