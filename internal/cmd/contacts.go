package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

const (
	contactsDefaultPage = 0
	contactsDefaultSize = 20
	contactLabelWidth   = 14
)

var (
	contactsListQueryFlag string
	contactsListPageFlag  int
	contactsListSizeFlag  int
	contactsImportFile    string
	contactsSearchQuery   string
)

var contactsCmd = &cobra.Command{
	Use:   "contacts",
	Short: "Manage contacts",
	Long:  "List, search, import, and view statistics for your contacts.\nContacts are the recipients of your WhatsApp messages and campaigns.",
}

var contactsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all contacts",
	RunE:  runContactsList,
}

var contactsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search contacts by name or phone",
	RunE:  runContactsSearch,
}

var contactsImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import contacts from a JSON file",
	RunE:  runContactsImport,
}

var contactsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show contact statistics",
	RunE:  runContactsStats,
}

func init() {
	contactsListCmd.Flags().StringVarP(&contactsListQueryFlag, "query", "q", "", "search query to filter contacts")
	contactsListCmd.Flags().IntVar(&contactsListPageFlag, "page", contactsDefaultPage, "page number (zero-indexed)")
	contactsListCmd.Flags().IntVar(&contactsListSizeFlag, "size", contactsDefaultSize, "number of contacts per page")

	contactsSearchCmd.Flags().StringVarP(&contactsSearchQuery, "query", "q", "", "search query (required)")

	contactsImportCmd.Flags().StringVar(&contactsImportFile, "file", "", "path to JSON file containing contacts (required)")

	contactsCmd.AddCommand(contactsListCmd)
	contactsCmd.AddCommand(contactsSearchCmd)
	contactsCmd.AddCommand(contactsImportCmd)
	contactsCmd.AddCommand(contactsStatsCmd)

	rootCmd.AddCommand(contactsCmd)
}

func runContactsList(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runContactsListImpl(client, GetOutputFormat(), os.Stdout, contactsListQueryFlag, contactsListPageFlag, contactsListSizeFlag)
}

func runContactsListImpl(client *api.Client, format output.Format, writer io.Writer, query string, page int, size int) error {
	response, listError := client.ListContacts(query, page, size)
	if listError != nil {
		return fmt.Errorf("list contacts: %w", listError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, response)
	}

	if len(response.Contacts) == 0 {
		fmt.Fprintln(writer, "No contacts found.")
		return nil
	}

	printContactsTable(writer, response)
	return nil
}

func runContactsSearch(command *cobra.Command, arguments []string) error {
	if contactsSearchQuery == "" {
		searchError := fmt.Errorf("--query/-q is required for search")
		output.PrintError(searchError.Error())
		return searchError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	response, searchError := client.ListContacts(contactsSearchQuery, contactsDefaultPage, contactsDefaultSize)
	if searchError != nil {
		output.PrintError(fmt.Sprintf("Failed to search contacts: %s", searchError.Error()))
		return searchError
	}

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(response)
	}

	if len(response.Contacts) == 0 {
		output.PrintInfo(fmt.Sprintf("No contacts found matching %q", contactsSearchQuery))
		return nil
	}

	printContactsTable(os.Stdout, response)

	return nil
}

func runContactsImport(command *cobra.Command, arguments []string) error {
	if contactsImportFile == "" {
		importError := fmt.Errorf("--file is required for contact import")
		output.PrintError(importError.Error())
		return importError
	}

	fileBytes, readError := os.ReadFile(contactsImportFile)
	if readError != nil {
		output.PrintError(fmt.Sprintf("Failed to read file %q: %s", contactsImportFile, readError.Error()))
		return readError
	}

	contacts, parseError := parseContactsImport(fileBytes)
	if parseError != nil {
		output.PrintError(parseError.Error())
		return parseError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runContactsImportImpl(client, GetOutputFormat(), os.Stdout, contacts)
}

func runContactsImportImpl(client *api.Client, format output.Format, writer io.Writer, contacts []api.ContactRequest) error {
	importID, importError := client.ImportContacts(contacts)
	if importError != nil {
		return fmt.Errorf("import contacts: %w", importError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, map[string]any{
			"importId": importID,
			"count":    len(contacts),
		})
	}

	fmt.Fprintf(writer, "Imported %d contacts successfully (import ID: %s)\n", len(contacts), importID)
	return nil
}

func runContactsStats(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runContactsStatsImpl(client, GetOutputFormat(), os.Stdout)
}

func runContactsStatsImpl(client *api.Client, format output.Format, writer io.Writer) error {
	stats, statsError := client.GetContactStats()
	if statsError != nil {
		return fmt.Errorf("get contact stats: %w", statsError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, stats)
	}

	printContactStats(writer, stats)
	return nil
}

func parseContactsImport(fileBytes []byte) ([]api.ContactRequest, error) {
	var contacts []api.ContactRequest
	if unmarshalError := json.Unmarshal(fileBytes, &contacts); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse contacts JSON: %w", unmarshalError)
	}

	if len(contacts) == 0 {
		return nil, fmt.Errorf("contacts file is empty or contains no valid contacts")
	}

	return contacts, nil
}

func contactsTableData(response *api.ContactsListResponse) ([]string, [][]string, string) {
	headers := []string{"NAME", "PHONE", "EMAIL", "CREATED"}
	rows := make([][]string, 0, len(response.Contacts))

	for _, contact := range response.Contacts {
		email := contact.Email
		if email == "" {
			email = "-"
		}

		createdAt := contact.CreatedAt
		if len(createdAt) > 10 {
			createdAt = createdAt[:10]
		}

		rows = append(rows, []string{
			contact.Name,
			contact.Phone,
			email,
			createdAt,
		})
	}

	footer := fmt.Sprintf("Page %d of %d (total: %d contacts)", response.Page+1, response.TotalPages, response.Total)
	return headers, rows, footer
}

func printContactsTable(out io.Writer, response *api.ContactsListResponse) {
	headers, rows, footer := contactsTableData(response)
	output.WriteTable(out, headers, rows)

	fmt.Fprintf(out, "\n%s\n", output.DimStyle.Render(footer))
}

func printContactStats(out io.Writer, stats map[string]int64) {
	fmt.Fprintf(out, "  %-*s %d\n", contactLabelWidth, "Total:", stats["total"])
	fmt.Fprintf(out, "  %-*s %d\n", contactLabelWidth, "Active:", stats["active"])
	fmt.Fprintf(out, "  %-*s %d\n", contactLabelWidth, "Opted-out:", stats["optedOut"])
}
