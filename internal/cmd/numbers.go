package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

var numbersCmd = &cobra.Command{
	Use:   "numbers",
	Short: "List registered phone numbers",
	Long:  "List all WhatsApp phone numbers registered in your AraraHQ account.",
	RunE:  runNumbers,
}

func init() {
	rootCmd.AddCommand(numbersCmd)
}

func runNumbers(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runNumbersImpl(client, GetOutputFormat(), os.Stdout)
}

func runNumbersImpl(client *api.Client, format output.Format, writer io.Writer) error {
	numbers, listError := client.ListNumbers()
	if listError != nil {
		return fmt.Errorf("list phone numbers: %w", listError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, numbers)
	}

	if len(numbers) == 0 {
		fmt.Fprintln(writer, "No phone numbers found. Register one at ararahq.com/dashboard")
		return nil
	}

	writeNumbersTable(writer, numbers)
	return nil
}

func printNumbersTable(numbers []api.PhoneNumberResponse) {
	writeNumbersTable(os.Stdout, numbers)
}

func writeNumbersTable(writer io.Writer, numbers []api.PhoneNumberResponse) {
	headers, rows := numbersTableData(numbers)
	output.WriteTable(writer, headers, rows)
}

func numbersTableData(numbers []api.PhoneNumberResponse) ([]string, [][]string) {
	headers := []string{"NUMBER", "NAME", "ID"}
	rows := make([][]string, 0, len(numbers))

	for _, number := range numbers {
		rows = append(rows, []string{
			number.PhoneNumber,
			number.Name,
			number.ID,
		})
	}

	return headers, rows
}
