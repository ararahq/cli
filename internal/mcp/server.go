// Package mcp turns the AraraHQ CLI into a Model Context Protocol server.
// Agents (Claude Code, Cursor, custom MCP clients) can call a curated subset
// of the AraraHQ API as tools without re-implementing auth, retry, or
// idempotency — those concerns live in the underlying *api.Client.
package mcp

import (
	"context"
	"errors"
	"io"
	"log"
	"os"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/version"
)

const (
	serverName              = "arara-cli"
	logPrefix               = "[mcp] "
	logFlags                = log.LstdFlags | log.LUTC
	defaultDescriptionLimit = 200
)

// ErrNoAPIClient is returned when NewServer receives a nil *api.Client.
var ErrNoAPIClient = errors.New("mcp server requires a non-nil api.Client")

// Options controls how the MCP server behaves.
type Options struct {
	// AllowWriteTools opts the server into exposing tools that mutate AraraHQ
	// state (send_message, create_campaign, ...). Disabled by default; agents
	// that don't get a confirmation from the human shouldn't be sending
	// WhatsApp messages on the user's behalf.
	AllowWriteTools bool

	// Logger receives operational logs. Defaults to a stderr logger. Must NOT
	// write to stdout — stdout is reserved for the JSON-RPC stream.
	Logger *log.Logger
}

// Server hosts the MCP protocol over stdio (today) using mark3labs/mcp-go.
type Server struct {
	apiClient       *api.Client
	mcpServer       *mcpserver.MCPServer
	logger          *log.Logger
	allowWriteTools bool
	registered      []string
}

// NewServer wires the underlying mcp-go server with the AraraHQ API client.
// It registers tools immediately so callers can introspect the surface via
// ListToolNames before calling ServeStdio.
func NewServer(apiClient *api.Client, options Options) (*Server, error) {
	if apiClient == nil {
		return nil, ErrNoAPIClient
	}

	logger := options.Logger
	if logger == nil {
		logger = log.New(os.Stderr, logPrefix, logFlags)
	}

	server := &Server{
		apiClient:       apiClient,
		logger:          logger,
		allowWriteTools: options.AllowWriteTools,
	}

	server.mcpServer = mcpserver.NewMCPServer(
		serverName,
		version.Version,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery(),
		mcpserver.WithInputSchemaValidation(),
	)

	server.registerReadTools()
	if server.allowWriteTools {
		server.registerWriteTools()
	}

	return server, nil
}

// ListToolNames returns the names of registered tools in registration order.
// Useful for logging and tests.
func (server *Server) ListToolNames() []string {
	out := make([]string, len(server.registered))
	copy(out, server.registered)
	return out
}

// MCPServer exposes the underlying mcp-go server for advanced wiring (tests).
func (server *Server) MCPServer() *mcpserver.MCPServer {
	return server.mcpServer
}

// ServeStdio blocks reading JSON-RPC from stdin and writing to stdout until
// stdin closes or the context is canceled.
func (server *Server) ServeStdio(_ context.Context) error {
	return mcpserver.ServeStdio(
		server.mcpServer,
		mcpserver.WithErrorLogger(server.logger),
	)
}

func (server *Server) registerTool(tool mcpsdk.Tool, handler mcpserver.ToolHandlerFunc) {
	wrapped := server.instrumentHandler(tool.Name, handler)
	server.mcpServer.AddTool(tool, wrapped)
	server.registered = append(server.registered, tool.Name)
}

func (server *Server) instrumentHandler(toolName string, handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		result, callErr := handler(ctx, request)
		if callErr != nil {
			server.logger.Printf("tool=%s status=server-error err=%v", toolName, callErr)
			return result, callErr
		}
		if result != nil && result.IsError {
			server.logger.Printf("tool=%s status=tool-error", toolName)
		} else {
			server.logger.Printf("tool=%s status=ok", toolName)
		}
		return result, nil
	}
}

// discardLogger returns a logger that writes nowhere; useful in tests when
// stderr is noisy.
func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
