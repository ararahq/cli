package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

const (
	templateStatusApproved = "APPROVED"
	templateStatusPending  = "PENDING"
	templateStatusRejected = "REJECTED"

	tabWriterMinWidth = 0
	tabWriterTabWidth = 4
	tabWriterPadding  = 3
	tabWriterPadChar  = ' '
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Manage WhatsApp message templates",
	Long:  "List, inspect status, and delete WhatsApp message templates\nregistered in your AraraHQ account.",
}

var templatesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all message templates",
	RunE:  runTemplatesList,
}

var templatesStatusCmd = &cobra.Command{
	Use:   "status [template-id]",
	Short: "Check the approval status of a template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplatesStatus,
}

var templatesDeleteCmd = &cobra.Command{
	Use:   "delete [template-id]",
	Short: "Delete a message template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplatesDelete,
}

func init() {
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesStatusCmd)
	templatesCmd.AddCommand(templatesDeleteCmd)

	rootCmd.AddCommand(templatesCmd)
}

func runTemplatesList(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runTemplatesListImpl(client, GetOutputFormat(), os.Stdout)
}

// runTemplatesListImpl is the testable core of `arara templates list`: it
// owns the API call and the format-aware rendering but accepts the client
// and writer as inputs so tests can drive it with httptest + a buffer.
func runTemplatesListImpl(client *api.Client, format output.Format, writer io.Writer) error {
	templates, listError := client.ListTemplates()
	if listError != nil {
		return fmt.Errorf("list templates: %w", listError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, templates)
	}

	if len(templates) == 0 {
		fmt.Fprintln(writer, "No templates found. Create one at ararahq.com/dashboard")
		return nil
	}

	printTemplatesTable(writer, templates)
	return nil
}

func runTemplatesStatus(command *cobra.Command, arguments []string) error {
	templateID := arguments[0]

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runTemplatesStatusImpl(client, GetOutputFormat(), os.Stdout, templateID)
}

func runTemplatesStatusImpl(client *api.Client, format output.Format, writer io.Writer, templateID string) error {
	statusResponse, statusError := client.GetTemplateStatus(templateID)
	if statusError != nil {
		return fmt.Errorf("get template status: %w", statusError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, statusResponse)
	}

	printTemplateStatus(writer, templateID, statusResponse)
	return nil
}

func runTemplatesDelete(command *cobra.Command, arguments []string) error {
	templateID := arguments[0]

	if !confirmDeletion(fmt.Sprintf("Are you sure you want to delete template %q? [y/N] ", templateID)) {
		output.PrintInfo("Deletion cancelled.")
		return nil
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	if deleteError := client.DeleteTemplate(templateID); deleteError != nil {
		output.PrintError(fmt.Sprintf("Failed to delete template: %s", deleteError.Error()))
		return deleteError
	}

	output.PrintSuccess(fmt.Sprintf("Template %q deleted successfully.", templateID))

	return nil
}

func printTemplatesTable(out io.Writer, templates []api.Template) {
	writer := tabwriter.NewWriter(out, tabWriterMinWidth, tabWriterTabWidth, tabWriterPadding, tabWriterPadChar, 0)

	fmt.Fprintln(writer, "NAME\tCATEGORY\tSTATUS\tLANGUAGE")
	fmt.Fprintln(writer, "----\t--------\t------\t--------")

	for _, template := range templates {
		styledStatus := colorizeTemplateStatus(template.ProviderStatus)

		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
			template.Name,
			template.Category,
			styledStatus,
			template.Language,
		)
	}

	writer.Flush()
}

func printTemplateStatus(out io.Writer, templateID string, statusResponse *api.TemplateStatusResponse) {
	styledStatus := colorizeTemplateStatus(statusResponse.Status)

	fmt.Fprintf(out, "Template:  %s\n", templateID)
	fmt.Fprintf(out, "Status:    %s\n", styledStatus)
	fmt.Fprintf(out, "Category:  %s\n", statusResponse.Category)

	if statusResponse.RejectionReason != "" {
		fmt.Fprintf(out, "Reason:    %s\n", output.ErrorStyle.Render(statusResponse.RejectionReason))
	}
}

func colorizeTemplateStatus(status string) string {
	upperStatus := strings.ToUpper(status)

	switch upperStatus {
	case templateStatusApproved:
		return output.SuccessStyle.Render(upperStatus)
	case templateStatusPending:
		return output.WarningStyle.Render(upperStatus)
	case templateStatusRejected:
		return output.ErrorStyle.Render(upperStatus)
	default:
		return output.DimStyle.Render(upperStatus)
	}
}

func confirmDeletion(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))

	return answer == "y" || answer == "yes"
}
