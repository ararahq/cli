package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ararahq/cli/internal/api"
)

const (
	logsViewerHeight       = 30
	logsViewerWidth        = 120
	logsStatusBarHeight    = 2
	logsTimestampFormat    = "2006-01-02 15:04:05"
	logsStatusLabelTailing = "Tailing live events"
	logsStatusIcon         = "\u26A1"
	logsDisconnectedIcon   = "\u2717"
	logsDisconnectedLabel  = "Disconnected"
	logsQuitHint           = "q: quit"
)

var (
	logsTimestampStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	logsEventStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	logsReceiverStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	logsStatusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C853"))
	logsConnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C853"))
	logsDisconnStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1744"))
	logsCounterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	logsHintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
)

// logsEventReceived carries an SSE event for the logs viewer.
type logsEventReceived struct {
	event api.SSEEvent
}

// logsStreamDisconnected signals a stream disconnection.
type logsStreamDisconnected struct{}

// LogsViewerConfig holds configuration for the live logs viewer.
type LogsViewerConfig struct {
	Client *api.Client
}

// LogsViewerModel is the Bubbletea model for streaming logs.
type LogsViewerModel struct {
	config     LogsViewerConfig
	viewport   viewport.Model
	lines      []string
	eventCount int
	connected  bool
	ready      bool
}

// NewLogsViewer creates a new LogsViewerModel.
func NewLogsViewer(config LogsViewerConfig) LogsViewerModel {
	viewportModel := viewport.New(logsViewerWidth, logsViewerHeight)
	viewportModel.SetContent("")

	return LogsViewerModel{
		config:   config,
		viewport: viewportModel,
		lines:    make([]string, 0),
	}
}

// Init starts the SSE stream for logs.
func (model LogsViewerModel) Init() tea.Cmd {
	return model.subscribeToLogStream()
}

// Update handles messages for the logs viewer.
func (model LogsViewerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMessage := message.(type) {
	case tea.KeyMsg:
		return model.handleKeyPress(typedMessage)

	case tea.WindowSizeMsg:
		return model.handleWindowResize(typedMessage)

	case logsEventReceived:
		return model.handleEvent(typedMessage)

	case logsStreamDisconnected:
		return model.handleDisconnection()
	}

	updatedViewport, viewportCmd := model.viewport.Update(message)
	model.viewport = updatedViewport
	return model, viewportCmd
}

// View renders the logs viewer TUI.
func (model LogsViewerModel) View() string {
	var builder strings.Builder

	builder.WriteString(model.renderStatusBar())
	builder.WriteString("\n")
	builder.WriteString(model.viewport.View())
	builder.WriteString("\n")
	builder.WriteString(logsHintStyle.Render("  " + logsQuitHint))

	return builder.String()
}

func (model LogsViewerModel) handleKeyPress(keyMessage tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMessage.String() {
	case "q", "ctrl+c":
		return model, tea.Quit
	}

	updatedViewport, viewportCmd := model.viewport.Update(keyMessage)
	model.viewport = updatedViewport
	return model, viewportCmd
}

func (model *LogsViewerModel) handleWindowResize(sizeMessage tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	model.viewport.Width = sizeMessage.Width
	model.viewport.Height = sizeMessage.Height - logsStatusBarHeight
	model.ready = true
	model.viewport.SetContent(strings.Join(model.lines, "\n"))
	return model, nil
}

func (model *LogsViewerModel) handleEvent(received logsEventReceived) (tea.Model, tea.Cmd) {
	model.connected = true
	model.eventCount++

	logLine := formatLogLine(received.event)
	model.lines = append(model.lines, logLine)
	model.viewport.SetContent(strings.Join(model.lines, "\n"))
	model.viewport.GotoBottom()

	return model, model.waitForLogEvent()
}

func (model *LogsViewerModel) handleDisconnection() (tea.Model, tea.Cmd) {
	model.connected = false
	return model, model.subscribeToLogStream()
}

func (model LogsViewerModel) subscribeToLogStream() tea.Cmd {
	return func() tea.Msg {
		streamContext := context.Background()

		eventChannel, streamError := model.config.Client.StreamEvents(streamContext)
		if streamError != nil {
			return logsStreamDisconnected{}
		}

		event, channelOpen := <-eventChannel
		if !channelOpen {
			return logsStreamDisconnected{}
		}

		return logsEventReceived{event: event}
	}
}

func (model LogsViewerModel) waitForLogEvent() tea.Cmd {
	return func() tea.Msg {
		streamContext := context.Background()

		eventChannel, streamError := model.config.Client.StreamEvents(streamContext)
		if streamError != nil {
			return logsStreamDisconnected{}
		}

		event, channelOpen := <-eventChannel
		if !channelOpen {
			return logsStreamDisconnected{}
		}

		return logsEventReceived{event: event}
	}
}

func (model LogsViewerModel) renderStatusBar() string {
	if model.connected {
		statusText := logsConnStyle.Render(fmt.Sprintf("%s %s", logsStatusIcon, logsStatusLabelTailing))
		counterText := logsCounterStyle.Render(fmt.Sprintf("Events: %d", model.eventCount))
		return fmt.Sprintf("%s    %s", statusText, counterText)
	}

	return logsDisconnStyle.Render(fmt.Sprintf("%s %s", logsDisconnectedIcon, logsDisconnectedLabel))
}

func formatLogLine(event api.SSEEvent) string {
	timestamp := time.Now().Format(logsTimestampFormat)
	receiver := extractReceiverFromData(event.Data)
	status := extractStatusFromData(event.Data)

	return fmt.Sprintf(
		"%s  %s  %s  %s",
		logsTimestampStyle.Render(timestamp),
		logsEventStyle.Render(padLogEventType(event.Event)),
		logsReceiverStyle.Render(receiver),
		logsStatusStyle.Render(status),
	)
}

func extractReceiverFromData(data string) string {
	var payload map[string]any
	if jsonError := json.Unmarshal([]byte(data), &payload); jsonError != nil {
		return ""
	}

	receiver := extractStringField(payload, "receiver")
	if receiver == "" {
		receiver = extractStringField(payload, "to")
	}

	return maskPhoneNumber(receiver)
}

func extractStatusFromData(data string) string {
	var payload map[string]any
	if jsonError := json.Unmarshal([]byte(data), &payload); jsonError != nil {
		return ""
	}

	return extractStringField(payload, "status")
}

func padLogEventType(eventType string) string {
	const logEventTypePadWidth = 22
	if len(eventType) >= logEventTypePadWidth {
		return eventType
	}
	return eventType + strings.Repeat(" ", logEventTypePadWidth-len(eventType))
}
