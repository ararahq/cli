package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

const (
	campaignStatusProcessing = "PROCESSING"
	campaignStatusCompleted  = "COMPLETED"
	campaignStatusFailed     = "FAILED"
	campaignStatusPending    = "PENDING"
	campaignStatusCancelled  = "CANCELLED"

	campaignResultLabelWidth = 18
	percentageMultiplier     = 100.0
	contactsFilePrefix       = "@"
)

var (
	campaignCreateNameFlag     string
	campaignCreateTemplateFlag string
	campaignCreateContactsFlag string
	campaignEstimateTemplate   string
	campaignEstimateCount      int
)

var campaignsCmd = &cobra.Command{
	Use:   "campaigns",
	Short: "Manage WhatsApp campaigns",
	Long:  "Create, list, and monitor WhatsApp message campaigns.\nCampaigns allow bulk message sending to multiple recipients using templates.",
}

var campaignsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all campaigns",
	RunE:  runCampaignsList,
}

var campaignsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new campaign",
	RunE:  runCampaignsCreate,
}

var campaignsStatusCmd = &cobra.Command{
	Use:   "status [campaign-id]",
	Short: "Check campaign status and progress",
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignsStatus,
}

var campaignsEstimateCmd = &cobra.Command{
	Use:   "estimate",
	Short: "Estimate campaign cost before sending",
	RunE:  runCampaignsEstimate,
}

func init() {
	campaignsCreateCmd.Flags().StringVar(&campaignCreateNameFlag, "name", "", "campaign name (required)")
	campaignsCreateCmd.Flags().StringVar(&campaignCreateTemplateFlag, "template", "", "template name to use (required)")
	campaignsCreateCmd.Flags().StringVar(&campaignCreateContactsFlag, "contacts", "", "contacts as JSON string or @filepath (required)")

	campaignsEstimateCmd.Flags().StringVar(&campaignEstimateTemplate, "template", "", "template name (required)")
	campaignsEstimateCmd.Flags().IntVar(&campaignEstimateCount, "count", 0, "number of recipients (required)")

	campaignsCmd.AddCommand(campaignsListCmd)
	campaignsCmd.AddCommand(campaignsCreateCmd)
	campaignsCmd.AddCommand(campaignsStatusCmd)
	campaignsCmd.AddCommand(campaignsEstimateCmd)

	rootCmd.AddCommand(campaignsCmd)
}

func runCampaignsList(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runCampaignsListImpl(client, GetOutputFormat(), os.Stdout)
}

func runCampaignsListImpl(client *api.Client, format output.Format, writer io.Writer) error {
	campaigns, listError := client.ListCampaigns()
	if listError != nil {
		return fmt.Errorf("list campaigns: %w", listError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, campaigns)
	}

	if len(campaigns) == 0 {
		fmt.Fprintln(writer, "No campaigns found. Create one with 'arara campaigns create'")
		return nil
	}

	printCampaignsTable(writer, campaigns)
	return nil
}

func runCampaignsCreate(command *cobra.Command, arguments []string) error {
	if validationError := validateCampaignCreateFlags(); validationError != nil {
		output.PrintError(validationError.Error())
		return validationError
	}

	contacts, parseError := parseCampaignContacts(campaignCreateContactsFlag)
	if parseError != nil {
		output.PrintError(fmt.Sprintf("Failed to parse contacts: %s", parseError.Error()))
		return parseError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	request := api.CreateCampaignRequest{
		Name:         campaignCreateNameFlag,
		TemplateName: campaignCreateTemplateFlag,
		Contacts:     contacts,
	}

	return runCampaignsCreateImpl(client, GetOutputFormat(), os.Stdout, request, uuid.New().String())
}

func runCampaignsCreateImpl(client *api.Client, format output.Format, writer io.Writer, request api.CreateCampaignRequest, idempotencyKey string) error {
	response, createError := client.CreateCampaign(request, idempotencyKey)
	if createError != nil {
		return fmt.Errorf("create campaign: %w", createError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, response)
	}

	printCampaignCreated(writer, response)
	return nil
}

func runCampaignsStatus(command *cobra.Command, arguments []string) error {
	campaignID := arguments[0]

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runCampaignsStatusImpl(client, GetOutputFormat(), os.Stdout, campaignID)
}

func runCampaignsStatusImpl(client *api.Client, format output.Format, writer io.Writer, campaignID string) error {
	campaign, getError := client.GetCampaign(campaignID)
	if getError != nil {
		return fmt.Errorf("get campaign status: %w", getError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, campaign)
	}

	printCampaignStatus(writer, campaign)
	return nil
}

func runCampaignsEstimate(command *cobra.Command, arguments []string) error {
	if campaignEstimateTemplate == "" {
		estimateError := fmt.Errorf("--template is required for campaign estimate")
		output.PrintError(estimateError.Error())
		return estimateError
	}

	if campaignEstimateCount <= 0 {
		estimateError := fmt.Errorf("--count must be greater than zero")
		output.PrintError(estimateError.Error())
		return estimateError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	return runCampaignsEstimateImpl(client, GetOutputFormat(), os.Stdout, campaignEstimateTemplate, campaignEstimateCount)
}

func runCampaignsEstimateImpl(client *api.Client, format output.Format, writer io.Writer, templateName string, recipientCount int) error {
	estimate, estimateError := client.EstimateCampaign(templateName, recipientCount)
	if estimateError != nil {
		return fmt.Errorf("estimate campaign: %w", estimateError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, estimate)
	}

	printCampaignEstimate(writer, estimate)
	return nil
}

func validateCampaignCreateFlags() error {
	if campaignCreateNameFlag == "" {
		return fmt.Errorf("--name is required for campaign creation")
	}

	if campaignCreateTemplateFlag == "" {
		return fmt.Errorf("--template is required for campaign creation")
	}

	if campaignCreateContactsFlag == "" {
		return fmt.Errorf("--contacts is required for campaign creation (JSON string or @filepath)")
	}

	return nil
}

func parseCampaignContacts(contactsInput string) ([]api.CampaignContact, error) {
	rawJSON := contactsInput

	if strings.HasPrefix(contactsInput, contactsFilePrefix) {
		filePath := strings.TrimPrefix(contactsInput, contactsFilePrefix)

		fileBytes, readError := os.ReadFile(filePath)
		if readError != nil {
			return nil, fmt.Errorf("failed to read contacts file %q: %w", filePath, readError)
		}

		rawJSON = string(fileBytes)
	}

	var contacts []api.CampaignContact
	if unmarshalError := json.Unmarshal([]byte(rawJSON), &contacts); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse contacts JSON: %w", unmarshalError)
	}

	if len(contacts) == 0 {
		return nil, fmt.Errorf("contacts list is empty")
	}

	return contacts, nil
}

func printCampaignsTable(writer io.Writer, campaigns []api.CampaignResponse) {
	headers, rows := campaignsTableData(campaigns)
	output.WriteTable(writer, headers, rows)
}

func campaignsTableData(campaigns []api.CampaignResponse) ([]string, [][]string) {
	headers := []string{"NAME", "STATUS", "MESSAGES", "FAILED", "COST"}
	rows := make([][]string, 0, len(campaigns))

	for _, campaign := range campaigns {
		styledStatus := colorizeCampaignStatus(campaign.Status)
		rows = append(rows, []string{
			campaign.Name,
			styledStatus,
			fmt.Sprintf("%d", campaign.TotalMessages),
			fmt.Sprintf("%d", campaign.FailedCount()),
			fmt.Sprintf("$%.2f", campaign.TotalCost),
		})
	}

	return headers, rows
}

func printCampaignCreated(out io.Writer, campaign *api.CampaignResponse) {
	output.PrintSuccess("Campaign created successfully!")

	fmt.Fprintf(
		out,
		"  %-*s %s\n  %-*s %s\n  %-*s %s\n  %-*s %d\n  %-*s $%.2f\n",
		campaignResultLabelWidth, "ID:", campaign.ID,
		campaignResultLabelWidth, "Name:", campaign.Name,
		campaignResultLabelWidth, "Status:", colorizeCampaignStatus(campaign.Status),
		campaignResultLabelWidth, "Total Messages:", campaign.TotalMessages,
		campaignResultLabelWidth, "Estimated Cost:", campaign.TotalCost,
	)
}

func printCampaignStatus(out io.Writer, campaign *api.CampaignResponse) {
	fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "Campaign:", campaign.Name)
	fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "ID:", campaign.ID)
	fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "Status:", colorizeCampaignStatus(campaign.Status))

	if campaign.TotalMessages > 0 {
		progressPercentage := float64(campaign.SentCount) / float64(campaign.TotalMessages) * percentageMultiplier
		progressText := fmt.Sprintf("Processing: %d/%d messages (%.1f%%)", campaign.SentCount, campaign.TotalMessages, progressPercentage)
		fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "Progress:", progressText)
	}

	if campaign.FailedCount() > 0 {
		failedText := output.ErrorStyle.Render(fmt.Sprintf("%d failed", campaign.FailedCount()))
		fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "Failures:", failedText)
	}

	fmt.Fprintf(out, "  %-*s $%.2f\n", campaignResultLabelWidth, "Total Cost:", campaign.TotalCost)
}

func printCampaignEstimate(out io.Writer, estimate *api.CampaignEstimateResponse) {
	output.PrintSuccess("Campaign cost estimate:")

	fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "Template Category:", estimate.TemplateCategory)
	fmt.Fprintf(out, "  %-*s %d\n", campaignResultLabelWidth, "Recipients:", estimate.RecipientCount)
	fmt.Fprintf(out, "  %-*s $%.4f\n", campaignResultLabelWidth, "Template Cost:", estimate.TemplateCost)
	fmt.Fprintf(out, "  %-*s $%.4f\n", campaignResultLabelWidth, "Arara Fee:", estimate.AraraFee)
	fmt.Fprintf(out, "  %-*s $%.4f\n", campaignResultLabelWidth, "Unit Price:", estimate.UnitPrice)
	fmt.Fprintf(out, "  %-*s %s\n", campaignResultLabelWidth, "Total Cost:", output.BoldStyle.Render(fmt.Sprintf("$%.2f", estimate.TotalCost)))
}

func colorizeCampaignStatus(status string) string {
	upperStatus := strings.ToUpper(status)

	switch upperStatus {
	case campaignStatusCompleted:
		return output.SuccessStyle.Render(upperStatus)
	case campaignStatusProcessing:
		return output.WarningStyle.Render(upperStatus)
	case campaignStatusPending:
		return output.WarningStyle.Render(upperStatus)
	case campaignStatusFailed:
		return output.ErrorStyle.Render(upperStatus)
	case campaignStatusCancelled:
		return output.DimStyle.Render(upperStatus)
	default:
		return output.DimStyle.Render(upperStatus)
	}
}
