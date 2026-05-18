package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/mcp"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/settings"
)

const mcpLogPrefix = "[arara-mcp] "

var (
	mcpAllowWriteToolsFlag bool
	mcpDryRunFlag          bool
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as a Model Context Protocol server",
	Long: "Expose a curated subset of the AraraHQ API as MCP tools, callable by Claude Code,\n" +
		"Cursor, and other MCP clients. Reads JSON-RPC from stdin, writes responses to stdout,\n" +
		"logs to stderr. Reuses the active CLI profile's credentials.\n\n" +
		"Read tools are always exposed. Write tools (send_message, create_campaign, ...) are\n" +
		"gated by the 'mcp.allowWriteTools' setting or --allow-write-tools flag.",
	RunE: runMCPServer,
}

func init() {
	mcpCmd.Flags().BoolVar(&mcpAllowWriteToolsFlag, "allow-write-tools", false, "expose mutating tools (send_message, create_campaign, import_contacts)")
	mcpCmd.Flags().BoolVar(&mcpDryRunFlag, "dry-run", false, "print the list of registered tools and exit without serving")
	rootCmd.AddCommand(mcpCmd)
}

func runMCPServer(_ *cobra.Command, _ []string) error {
	allowWriteTools := resolveAllowWriteTools()
	logger := log.New(os.Stderr, mcpLogPrefix, log.LstdFlags|log.LUTC)

	apiClient, clientErr := resolveMCPAPIClient(mcpDryRunFlag)
	if clientErr != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientErr.Error()))
		return clientErr
	}
	apiClient.SetVerbose(IsVerbose())

	server, newErr := mcp.NewServer(apiClient, mcp.Options{
		AllowWriteTools: allowWriteTools,
		Logger:          logger,
	})
	if newErr != nil {
		output.PrintError(newErr.Error())
		return newErr
	}

	if mcpDryRunFlag {
		printRegisteredTools(server.ListToolNames(), allowWriteTools)
		return nil
	}

	logger.Printf("starting (allowWriteTools=%v tools=%d)", allowWriteTools, len(server.ListToolNames()))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if serveErr := server.ServeStdio(ctx); serveErr != nil {
		logger.Printf("serve error: %v", serveErr)
		return serveErr
	}

	logger.Printf("stopped")
	return nil
}

func resolveMCPAPIClient(dryRun bool) (*api.Client, error) {
	if dryRun {
		return api.NewClient(api.DefaultMCPDryRunURL, api.MCPDryRunKey), nil
	}
	return newClientForCmd()
}

func resolveAllowWriteTools() bool {
	if mcpAllowWriteToolsFlag {
		return true
	}
	resolved, loadErr := settings.Load("")
	if loadErr != nil || resolved == nil {
		return false
	}
	return resolved.Settings.MCP.AllowWriteTools
}

func printRegisteredTools(toolNames []string, allowWriteTools bool) {
	fmt.Fprintf(os.Stderr, "arara mcp — %d tools registered (allowWriteTools=%v)\n", len(toolNames), allowWriteTools)
	for _, name := range toolNames {
		fmt.Fprintf(os.Stderr, "  · %s\n", name)
	}
}
