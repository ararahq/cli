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

const orgLabelWidth = 18

var orgCmd = &cobra.Command{
	Use:   "org",
	Short: "Switch between organizations the authenticated user can access",
	Long: "Manage which organization (tenant) the CLI talks to. The user-level\n" +
		"credentials stay the same; only the active organization changes.\n\n" +
		"Useful for agencies/consultants operating multiple AraraHQ accounts\n" +
		"without re-running 'arara login' between them.",
}

var orgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List organizations accessible to the current user",
	RunE:  runOrgList,
}

var orgUseCmd = &cobra.Command{
	Use:   "use <slug>",
	Short: "Switch the active organization to the one with the given slug",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrgUse,
}

func init() {
	orgCmd.AddCommand(orgListCmd)
	orgCmd.AddCommand(orgUseCmd)
	rootCmd.AddCommand(orgCmd)
}

// orgRow is the display shape used by both text and json output. Mirrors
// api.Organization plus an `active` flag derived from the local active
// profile slug so users see which org they're operating against.
type orgRow struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Mode   string `json:"mode"`
	Active bool   `json:"active"`
}

func runOrgList(_ *cobra.Command, _ []string) error {
	client, clientErr := newClientForCmd()
	if clientErr != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientErr.Error()))
		return clientErr
	}
	client.SetVerbose(IsVerbose())

	return runOrgListImpl(client, GetOutputFormat(), os.Stdout, GetProfile())
}

func runOrgListImpl(client *api.Client, format output.Format, writer io.Writer, activeProfile string) error {
	orgs, listErr := client.ListOrganizations()
	if listErr != nil {
		return fmt.Errorf("list organizations: %w", listErr)
	}

	rows := buildOrgRows(orgs, activeProfile)

	switch format {
	case output.FormatJSON:
		return writeJSONIndentedTo(writer, rows)
	case output.FormatStreamJSON:
		for _, row := range rows {
			if err := output.WriteJSONLine(writer, row); err != nil {
				return err
			}
		}
		return nil
	}

	if len(rows) == 0 {
		fmt.Fprintln(writer, "No organizations available for this user.")
		return nil
	}

	printOrgRows(writer, rows)
	return nil
}

func runOrgUse(_ *cobra.Command, arguments []string) error {
	targetSlug := strings.TrimSpace(arguments[0])
	if targetSlug == "" {
		return fmt.Errorf("org slug is required")
	}

	client, clientErr := newClientForCmd()
	if clientErr != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientErr.Error()))
		return clientErr
	}
	client.SetVerbose(IsVerbose())

	return runOrgUseImpl(client, targetSlug)
}

// runOrgUseImpl is the testable core: list orgs, find by slug, persist the
// switch by promoting that org's profile to current. Today there's a single
// API key per user, so changing org just relabels the active profile —
// we don't rotate credentials. When the backend ships per-org keys, this
// is where we'd swap them.
func runOrgUseImpl(client *api.Client, targetSlug string) error {
	orgs, listErr := client.ListOrganizations()
	if listErr != nil {
		return fmt.Errorf("list organizations: %w", listErr)
	}

	matchedOrg, found := findOrgBySlug(orgs, targetSlug)
	if !found {
		return fmt.Errorf("organization %q not found in the user's accessible orgs (%s)",
			targetSlug, joinOrgSlugs(orgs))
	}

	configuration, loadErr := config.Load()
	if loadErr != nil {
		return fmt.Errorf("load config: %w", loadErr)
	}

	promoteOrgToActiveProfile(configuration, matchedOrg)

	if saveErr := config.Save(configuration); saveErr != nil {
		return fmt.Errorf("save config: %w", saveErr)
	}

	output.PrintSuccess(fmt.Sprintf("Active organization: %s (%s)", matchedOrg.Slug, matchedOrg.Name))
	return nil
}

func buildOrgRows(orgs []api.Organization, activeProfile string) []orgRow {
	rows := make([]orgRow, 0, len(orgs))
	for _, org := range orgs {
		rows = append(rows, orgRow{
			ID:     org.ID,
			Slug:   org.Slug,
			Name:   org.Name,
			Role:   org.Role,
			Mode:   org.Mode,
			Active: org.Slug == activeProfile,
		})
	}
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Slug < rows[right].Slug
	})
	return rows
}

func printOrgRows(writer io.Writer, rows []orgRow) {
	for _, row := range rows {
		marker := " "
		display := row.Slug
		if row.Active {
			marker = output.SuccessStyle.Render("*")
			display = output.BoldStyle.Render(row.Slug)
		}
		fmt.Fprintf(writer, "%s %-*s %s  %s  %s\n",
			marker,
			orgLabelWidth, display,
			output.DimStyle.Render(row.Mode),
			output.DimStyle.Render(row.Role),
			row.Name,
		)
	}
}

func findOrgBySlug(orgs []api.Organization, slug string) (api.Organization, bool) {
	for _, org := range orgs {
		if org.Slug == slug {
			return org, true
		}
	}
	return api.Organization{}, false
}

func joinOrgSlugs(orgs []api.Organization) string {
	if len(orgs) == 0 {
		return "(none)"
	}
	slugs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		slugs = append(slugs, org.Slug)
	}
	sort.Strings(slugs)
	return strings.Join(slugs, ", ")
}

// promoteOrgToActiveProfile makes the chosen org the current_profile in
// config. If a profile with that slug exists, switch to it; otherwise
// duplicate the existing active profile under the slug name and switch.
// Reuses the local profile cascade so all subsequent commands see the new
// org context without further plumbing.
func promoteOrgToActiveProfile(configuration *config.Config, org api.Organization) {
	if configuration.Profiles == nil {
		configuration.Profiles = map[string]config.Profile{}
	}

	if _, exists := configuration.Profiles[org.Slug]; !exists {
		base := configuration.Profiles[configuration.CurrentProfile]
		configuration.Profiles[org.Slug] = config.Profile{
			APIKey:   base.APIKey,
			APIURL:   nonEmpty(base.APIURL, config.DefaultAPIURL),
			Mode:     mapOrgModeToProfileMode(org.Mode),
			AuthType: base.AuthType,
		}
	}

	configuration.CurrentProfile = org.Slug
}

func mapOrgModeToProfileMode(orgMode string) string {
	switch strings.ToLower(strings.TrimSpace(orgMode)) {
	case "test":
		return "test"
	case "live":
		return "live"
	default:
		return config.DefaultMode
	}
}
