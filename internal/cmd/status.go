package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/tui"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"dash", "dashboard"},
	Short:   "Show a real-time dashboard with metrics, messages, and wallet balance",
	Long: "Launches a full-screen TUI dashboard that displays your account metrics,\n" +
		"recent messages, and wallet balance. Auto-refreshes every 30 seconds.\n\n" +
		"Keybindings:\n" +
		"  r    Refresh data manually\n" +
		"  q    Quit the dashboard",
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(command *cobra.Command, arguments []string) error {
	apiClient, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	apiClient.SetVerbose(IsVerbose())

	configuration, configError := config.Load()
	if configError != nil {
		output.PrintError(fmt.Sprintf("Failed to load configuration: %s", configError.Error()))
		return configError
	}

	profileName := configuration.CurrentProfile
	mode := apiClient.Mode()

	dashboardModel := tui.NewDashboardModel(apiClient, profileName, mode)
	program := tea.NewProgram(dashboardModel, tea.WithAltScreen())

	_, runError := program.Run()
	if runError != nil {
		output.PrintError(fmt.Sprintf("Dashboard error: %s", runError.Error()))
		return runError
	}

	return nil
}
