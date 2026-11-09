package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

const (
	defaultLogsLimit    = 20
	defaultLogsPage     = 0
	logsMessageIDHeader = "ID"
	logsToHeader        = "TO"
	logsTemplateHeader  = "TEMPLATE"
	logsStatusHeader    = "STATUS"
	logsTimeHeader      = "TIME"
)

var (
	logsLimitFlag int
	logsModeFlag  string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View recent messages",
	Long:  "Fetch and display recent messages from the AraraHQ console.",
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().IntVar(&logsLimitFlag, "limit", defaultLogsLimit, "number of recent messages to display")
	logsCmd.Flags().StringVar(&logsModeFlag, "mode", "", "filter by mode (e.g., sandbox, production)")

	rootCmd.AddCommand(logsCmd)
}

func runLogs(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	return runLogsFetch(client)
}

func runLogsFetch(client *api.Client) error {
	mode := resolveLogsMode(client)
	return runLogsFetchImpl(client, GetOutputFormat(), os.Stdout, mode, logsLimitFlag)
}

func runLogsFetchImpl(client *api.Client, format output.Format, writer io.Writer, mode string, limit int) error {
	messagesResponse, fetchError := client.GetMessages(mode, defaultLogsPage, limit)
	if fetchError != nil {
		return fmt.Errorf("fetch messages: %w", fetchError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, messagesResponse)
	}

	headers, rows, found := messagesTableData(messagesResponse)
	if !found {
		fmt.Fprintln(writer, "No messages found.")
		return nil
	}
	output.WriteTable(writer, headers, rows)
	return nil
}

func resolveLogsMode(client *api.Client) string {
	if logsModeFlag != "" {
		return logsModeFlag
	}

	return client.Mode()
}

func printMessagesTable(response map[string]any) {
	headers, rows, found := messagesTableData(response)
	if !found {
		output.PrintInfo("No messages found.")
		return
	}

	output.PrintTable(headers, rows)
}

// messagesTableData converts the raw response shape into table-ready rows.
// Returns found=false when there are no messages so the caller can decide
// whether to render an empty-state message instead of an empty table.
func messagesTableData(response map[string]any) ([]string, [][]string, bool) {
	dataSlice, dataExists := response["data"]
	if !dataExists {
		return nil, nil, false
	}

	messages, isSlice := dataSlice.([]any)
	if !isSlice || len(messages) == 0 {
		return nil, nil, false
	}

	headers := []string{logsMessageIDHeader, logsToHeader, logsTemplateHeader, logsStatusHeader, logsTimeHeader}
	rows := make([][]string, 0, len(messages))

	for _, rawMessage := range messages {
		messageMap, isMap := rawMessage.(map[string]any)
		if !isMap {
			continue
		}
		rows = append(rows, buildMessageRow(messageMap))
	}

	return headers, rows, true
}

func buildMessageRow(messageMap map[string]any) []string {
	messageID := extractMapString(messageMap, "id")
	receiver := extractMapString(messageMap, "receiver")
	templateName := extractMapString(messageMap, "templateName")
	status := extractMapString(messageMap, "status")
	createdAt := extractMapString(messageMap, "createdAt")

	if templateName == "" {
		templateName = "-"
	}

	return []string{messageID, receiver, templateName, status, createdAt}
}

func extractMapString(data map[string]any, key string) string {
	value, exists := data[key]
	if !exists {
		return ""
	}

	stringValue, isString := value.(string)
	if !isString {
		return fmt.Sprintf("%v", value)
	}

	return stringValue
}
