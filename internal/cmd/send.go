package cmd

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/hooks"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/settings"
	"github.com/ararahq/cli/internal/tui"
)

const (
	sendResultLabelWidth = 12

	defaultWatchTimeout      = 60 * time.Second
	watchInitialPollInterval = 500 * time.Millisecond
	watchMaxPollInterval     = 5 * time.Second
	watchPollMultiplier      = 2
)

var terminalMessageStatuses = map[string]struct{}{
	"delivered":   {},
	"read":        {},
	"failed":      {},
	"undelivered": {},
	"sent":        {},
	"canceled":    {},
}

var failureMessageStatuses = map[string]struct{}{
	"failed":      {},
	"undelivered": {},
	"canceled":    {},
}

var (
	sendToFlag           string
	sendTemplateFlag     string
	sendBodyFlag         string
	sendVarsFlag         string
	sendScheduledAtFlag  string
	sendDryRunFlag       bool
	sendWatchFlag        bool
	sendWatchTimeoutFlag time.Duration
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a WhatsApp message",
	Long: "Send a WhatsApp message to a recipient using a template or freeform body text.\n" +
		"Phone numbers are automatically formatted with the whatsapp: channel prefix.\n\n" +
		"Use --dry-run to preview the payload + estimated cost without sending.\n" +
		"Use --watch to block until the message reaches a terminal status (delivered, read, failed).",
	RunE: runSend,
}

func init() {
	sendCmd.Flags().StringVar(&sendToFlag, "to", "", "recipient phone number (e.g., +5511999999999)")
	sendCmd.Flags().StringVar(&sendTemplateFlag, "template", "", "template name to use for the message")
	sendCmd.Flags().StringVar(&sendBodyFlag, "body", "", "freeform message body (mutually exclusive with --template)")
	sendCmd.Flags().StringVar(&sendVarsFlag, "vars", "", "comma-separated template variables (e.g., \"John,Doe,100\")")
	sendCmd.Flags().StringVar(&sendScheduledAtFlag, "scheduled-at", "", "schedule delivery at ISO 8601 timestamp (e.g., 2025-12-01T10:00:00Z)")
	sendCmd.Flags().BoolVar(&sendDryRunFlag, "dry-run", false, "preview payload + estimated cost without sending")
	sendCmd.Flags().BoolVar(&sendWatchFlag, "watch", false, "wait for terminal status (delivered/failed/...) and exit on it")
	sendCmd.Flags().DurationVar(&sendWatchTimeoutFlag, "watch-timeout", defaultWatchTimeout, "max duration to --watch a message")

	rootCmd.AddCommand(sendCmd)
}

func runSend(command *cobra.Command, arguments []string) error {
	if noSendFlagsProvided() {
		if sendDryRunFlag || sendWatchFlag {
			return fmt.Errorf("--dry-run and --watch require --to plus --template or --body")
		}
		return runSendWizard()
	}

	if validationError := validateSendFlags(); validationError != nil {
		output.PrintError(validationError.Error())
		return validationError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	request := buildSendMessageRequest()
	request.Receiver = api.NormalizeReceiver(request.Receiver)

	if sendDryRunFlag {
		return runSendDryRun(client, request)
	}

	resolvedSettings, _ := settings.Load("")

	if hookErr := runConfiguredHooks(hooks.EventPreSend, request, resolvedSettings); hookErr != nil {
		return hookErr
	}

	response, sendError := client.SendMessage(request)
	if sendError != nil {
		_ = runConfiguredHooks(hooks.EventOnError, map[string]any{
			"request": request,
			"error":   sendError.Error(),
		}, resolvedSettings)
		output.PrintError(fmt.Sprintf("Failed to send message: %s", sendError.Error()))
		return sendError
	}

	if hookErr := runConfiguredHooks(hooks.EventPostDeliver, response, resolvedSettings); hookErr != nil {
		output.PrintWarning(fmt.Sprintf("postDeliver hook reported issues: %s", hookErr.Error()))
	}

	if sendWatchFlag {
		return runSendWatch(client, response)
	}

	return printSendOutcome(response)
}

func runConfiguredHooks(event string, payload any, resolvedSettings *settings.Resolved) error {
	if resolvedSettings == nil {
		return nil
	}

	scripts := selectHookScripts(event, &resolvedSettings.Settings.Hooks)
	if len(scripts) == 0 {
		return nil
	}

	runner := hooks.NewRunner()
	if resolvedSettings.Settings.HookTimeoutSeconds > 0 {
		runner.Timeout = time.Duration(resolvedSettings.Settings.HookTimeoutSeconds) * time.Second
	}

	ctx := context.Background()
	_, runErr := runner.Run(ctx, scripts, hooks.Event{
		Name:    event,
		Profile: GetProfile(),
		Mode:    GetMode(),
		Payload: payload,
	})
	return runErr
}

func selectHookScripts(event string, configured *settings.Hooks) []string {
	if configured == nil {
		return nil
	}
	switch event {
	case hooks.EventPreSend:
		return configured.PreSend
	case hooks.EventPostDeliver:
		return configured.PostDeliver
	case hooks.EventPreCampaign:
		return configured.PreCampaign
	case hooks.EventPostCampaign:
		return configured.PostCampaign
	case hooks.EventOnError:
		return configured.OnError
	default:
		return nil
	}
}

func runSendDryRun(client *api.Client, request api.SendMessageRequest) error {
	preview := api.DryRunPreview{
		Method:  "POST",
		Path:    "/v1/messages",
		Payload: request,
	}

	if request.TemplateName != "" {
		estimate, estimateError := client.EstimateCampaign(request.TemplateName, 1)
		if estimateError == nil {
			preview.EstimatedCost = estimate
		} else {
			preview.EstimateError = estimateError.Error()
		}
	} else {
		preview.EstimateError = "cost estimate is only available for template messages"
	}

	if GetOutputFormat().IsMachineReadable() {
		if GetOutputFormat() == output.FormatStreamJSON {
			return output.PrintJSONLine(preview)
		}
		return output.PrintJSON(preview)
	}

	printDryRunPreview(preview)
	return nil
}

func runSendWatch(client *api.Client, initial *api.MessageResponse) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	deadline := time.Now().Add(sendWatchTimeoutFlag)

	emitWatchUpdate(initial)

	current := initial
	pollInterval := watchInitialPollInterval

	for {
		if isTerminalMessageStatus(current.Status) {
			if isFailureMessageStatus(current.Status) {
				return fmt.Errorf("message reached terminal failure status: %s", current.Status)
			}
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("watch timed out after %s — last status: %s", sendWatchTimeoutFlag, current.Status)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollInterval):
		}

		updated, statusError := client.GetMessageStatus(initial.ID)
		if statusError != nil {
			return fmt.Errorf("failed to fetch message status while watching: %w", statusError)
		}

		if updated.Status != current.Status {
			emitWatchUpdate(updated)
		}
		current = updated

		pollInterval = nextWatchInterval(pollInterval)
	}
}

func emitWatchUpdate(response *api.MessageResponse) {
	record := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"id":        response.ID,
		"status":    response.Status,
		"receiver":  response.Receiver,
	}
	if response.TemplateName != "" {
		record["templateName"] = response.TemplateName
	}

	if GetOutputFormat() == output.FormatStreamJSON {
		_ = output.PrintJSONLine(record)
		return
	}

	if GetOutputFormat() == output.FormatJSON {
		_ = output.PrintJSON(record)
		return
	}

	fmt.Fprintf(os.Stdout, "[%s] %s status=%s\n", record["timestamp"], response.ID, response.Status)
}

func nextWatchInterval(current time.Duration) time.Duration {
	doubled := current * watchPollMultiplier
	if doubled > watchMaxPollInterval {
		doubled = watchMaxPollInterval
	}
	jitter := time.Duration(rand.Float64() * float64(doubled) * 0.1) //nolint:gosec // non-cryptographic jitter
	return doubled + jitter
}

func isTerminalMessageStatus(status string) bool {
	_, exists := terminalMessageStatuses[strings.ToLower(strings.TrimSpace(status))]
	return exists
}

func isFailureMessageStatus(status string) bool {
	_, exists := failureMessageStatuses[strings.ToLower(strings.TrimSpace(status))]
	return exists
}

func noSendFlagsProvided() bool {
	return sendToFlag == "" && sendTemplateFlag == "" && sendBodyFlag == ""
}

func runSendWizard() error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	wizardModel := tui.NewSendWizardModel(client)
	program := tea.NewProgram(wizardModel, tea.WithAltScreen())

	_, runError := program.Run()
	if runError != nil {
		output.PrintError(fmt.Sprintf("Send wizard failed: %s", runError.Error()))
		return runError
	}

	return nil
}

func validateSendFlags() error {
	if sendToFlag == "" {
		return fmt.Errorf("--to is required — specify the recipient phone number")
	}

	if sendTemplateFlag == "" && sendBodyFlag == "" {
		return fmt.Errorf("either --template or --body is required")
	}

	if sendTemplateFlag != "" && sendBodyFlag != "" {
		return errors.New("--template and --body are mutually exclusive — use one or the other")
	}

	if sendDryRunFlag && sendWatchFlag {
		return errors.New("--dry-run and --watch are mutually exclusive")
	}

	return nil
}

func buildSendMessageRequest() api.SendMessageRequest {
	request := api.SendMessageRequest{
		Receiver:     sendToFlag,
		TemplateName: sendTemplateFlag,
		Body:         sendBodyFlag,
		ScheduledAt:  sendScheduledAtFlag,
	}

	if sendVarsFlag != "" {
		request.TemplateVariables = parseCommaSeparatedVars(sendVarsFlag)
	}

	return request
}

func parseCommaSeparatedVars(rawVars string) []string {
	parts := strings.Split(rawVars, ",")
	variables := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			variables = append(variables, trimmed)
		}
	}

	return variables
}

func printSendOutcome(response *api.MessageResponse) error {
	if GetOutputFormat() == output.FormatStreamJSON {
		return output.PrintJSONLine(response)
	}
	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(response)
	}

	printSendSuccess(response)
	return nil
}

func printSendSuccess(response *api.MessageResponse) {
	output.PrintSuccess("Message sent successfully!")

	templateDisplay := response.TemplateName
	if templateDisplay == "" {
		templateDisplay = "(freeform)"
	}

	receiverDisplay := extractPhoneNumber(response.Receiver)

	fmt.Fprintf(
		os.Stdout,
		"  %-*s %s\n  %-*s %s\n  %-*s %s\n  %-*s %s\n",
		sendResultLabelWidth, "ID:", response.ID,
		sendResultLabelWidth, "To:", receiverDisplay,
		sendResultLabelWidth, "Status:", response.Status,
		sendResultLabelWidth, "Template:", templateDisplay,
	)
}

func printDryRunPreview(preview api.DryRunPreview) {
	fmt.Fprintf(os.Stdout, "%s %s %s\n\n", output.DimStyle.Render("DRY RUN —"), preview.Method, preview.Path)

	output.PrintInfo("Payload (would be sent as JSON):")
	fmt.Fprintf(os.Stdout, "  to:           %s\n", preview.Payload.Receiver)
	if preview.Payload.TemplateName != "" {
		fmt.Fprintf(os.Stdout, "  template:     %s\n", preview.Payload.TemplateName)
		if len(preview.Payload.TemplateVariables) > 0 {
			fmt.Fprintf(os.Stdout, "  vars:         %s\n", strings.Join(preview.Payload.TemplateVariables, ", "))
		}
	}
	if preview.Payload.Body != "" {
		fmt.Fprintf(os.Stdout, "  body:         %s\n", preview.Payload.Body)
	}
	if preview.Payload.ScheduledAt != "" {
		fmt.Fprintf(os.Stdout, "  scheduled at: %s\n", preview.Payload.ScheduledAt)
	}

	fmt.Fprintln(os.Stdout)
	if preview.EstimatedCost != nil {
		output.PrintInfo("Estimated cost:")
		fmt.Fprintf(os.Stdout, "  template:     %s (%s)\n", preview.EstimatedCost.TemplateCategory, preview.Payload.TemplateName)
		fmt.Fprintf(os.Stdout, "  unit price:   %.4f\n", preview.EstimatedCost.UnitPrice)
		fmt.Fprintf(os.Stdout, "  total cost:   %.4f\n", preview.EstimatedCost.TotalCost)
	} else if preview.EstimateError != "" {
		output.PrintWarning(fmt.Sprintf("Cost estimate unavailable: %s", preview.EstimateError))
	}
}

func extractPhoneNumber(receiver string) string {
	const whatsappPrefix = "whatsapp:"

	if strings.HasPrefix(receiver, whatsappPrefix) {
		return strings.TrimPrefix(receiver, whatsappPrefix)
	}

	return receiver
}
