package cmd

import (
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
	defaultKeyMode    = "LIVE"
	keyWarningMessage = "Save this key -- you won't see it again!"
	keysResultPadding = 12
)

var keysCreateModeFlag string

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage API keys",
	Long:  "List, create, and revoke API keys for your AraraHQ account.\nAPI keys are used to authenticate requests to the AraraHQ API.",
}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all API keys",
	RunE:  runKeysList,
}

var keysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API key",
	RunE:  runKeysCreate,
}

var keysRevokeCmd = &cobra.Command{
	Use:   "revoke [key-id]",
	Short: "Revoke an API key",
	Args:  cobra.ExactArgs(1),
	RunE:  runKeysRevoke,
}

func init() {
	keysCreateCmd.Flags().StringVar(&keysCreateModeFlag, "mode", defaultKeyMode, "key mode: LIVE or TEST")

	keysCmd.AddCommand(keysListCmd)
	keysCmd.AddCommand(keysCreateCmd)
	keysCmd.AddCommand(keysRevokeCmd)

	rootCmd.AddCommand(keysCmd)
}

func runKeysList(command *cobra.Command, arguments []string) error {
	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runKeysListImpl(client, GetOutputFormat(), os.Stdout)
}

func runKeysListImpl(client *api.Client, format output.Format, writer io.Writer) error {
	apiKeys, listError := client.ListAPIKeys()
	if listError != nil {
		return fmt.Errorf("list API keys: %w", listError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, apiKeys)
	}

	if len(apiKeys) == 0 {
		fmt.Fprintln(writer, "No API keys found.")
		return nil
	}

	printAPIKeysTable(writer, apiKeys)
	return nil
}

func runKeysCreate(command *cobra.Command, arguments []string) error {
	normalizedMode := strings.ToUpper(strings.TrimSpace(keysCreateModeFlag))

	if validationError := validateKeyMode(normalizedMode, keysCreateModeFlag); validationError != nil {
		output.PrintError(validationError.Error())
		return validationError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return runKeysCreateImpl(client, GetOutputFormat(), os.Stdout, normalizedMode)
}

func runKeysCreateImpl(client *api.Client, format output.Format, writer io.Writer, normalizedMode string) error {
	generatedKey, createError := client.CreateAPIKey(normalizedMode)
	if createError != nil {
		return fmt.Errorf("create API key: %w", createError)
	}

	if format == output.FormatJSON {
		return writeJSONIndentedTo(writer, generatedKey)
	}

	printCreatedKey(writer, generatedKey, normalizedMode)
	return nil
}

func validateKeyMode(normalizedMode string, originalInput string) error {
	if normalizedMode == "LIVE" || normalizedMode == "TEST" {
		return nil
	}
	return fmt.Errorf("invalid mode %q -- use LIVE or TEST", originalInput)
}

func runKeysRevoke(command *cobra.Command, arguments []string) error {
	keyID := arguments[0]

	if !confirmDeletion(fmt.Sprintf("Are you sure you want to revoke API key %q? [y/N] ", keyID)) {
		output.PrintInfo("Revocation cancelled.")
		return nil
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	if revokeError := client.RevokeAPIKey(keyID); revokeError != nil {
		output.PrintError(fmt.Sprintf("Failed to revoke API key: %s", revokeError.Error()))
		return revokeError
	}

	output.PrintSuccess(fmt.Sprintf("API key %q revoked successfully.", keyID))

	return nil
}

func printAPIKeysTable(out io.Writer, apiKeys []api.APIKeyInfo) {
	writer := tabwriter.NewWriter(out, tabWriterMinWidth, tabWriterTabWidth, tabWriterPadding, tabWriterPadChar, 0)

	fmt.Fprintln(writer, "PREFIX\tLAST 4\tMODE\tCREATED\tLAST USED")
	fmt.Fprintln(writer, "------\t------\t----\t-------\t---------")

	for _, keyInfo := range apiKeys {
		lastUsed := keyInfo.LastUsedAt
		if lastUsed == "" {
			lastUsed = "never"
		}

		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			keyInfo.Prefix,
			keyInfo.LastFour,
			keyInfo.Mode,
			keyInfo.CreatedAt,
			lastUsed,
		)
	}

	writer.Flush()
}

func printCreatedKey(out io.Writer, generatedKey *api.GeneratedAPIKey, mode string) {
	output.PrintWarning(keyWarningMessage)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %-*s %s\n", keysResultPadding, "Mode:", mode)
	fmt.Fprintf(out, "  %-*s %s\n", keysResultPadding, "Key:", output.BoldStyle.Render(generatedKey.PlainTextKey))
}
