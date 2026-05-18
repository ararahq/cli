package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ararahq/cli/internal/api"
)

const (
	webhookViewerWidth       = 120
	webhookViewerHeight      = 30
	statusBarHeight          = 2
	forwardingTimeoutSeconds = 10
	httpStatusOKThreshold    = 300
	httpStatusClientError    = 400
	httpStatusServerError    = 500
	phoneMaskVisibleDigits   = 5
	phoneMaskPrefix          = "..."
	latencyUnitMilliseconds  = "ms"
	statusConnected          = "connected"
	statusDisconnected       = "disconnected"
	jsonIndentSpaces         = "  "
	statusLabelConnected     = "Connected to SSE stream"
	statusLabelDisconnected  = "Disconnected"
	statusIconConnected      = "\u26A1"
	statusIconDisconnected   = "\u2717"
	eventCounterLabel        = "Events received: "
	quitHint                 = "q: quit"
	jsonToggleHint           = "j: toggle JSON"
	whatsappReceiverPrefix   = "whatsapp:"
)

// Styles for the webhook viewer.
var (
	timestampStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	eventTypeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	phoneStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	statusOKStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C853"))
	statusErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF1744"))
	latencyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	connectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C853"))
	disconnStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1744"))
	counterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	hintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	jsonStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// webhookEventReceived is a Bubbletea message carrying a new SSE event.
type webhookEventReceived struct {
	event api.SSEEvent
}

// webhookStreamDisconnected signals the SSE stream closed unexpectedly.
type webhookStreamDisconnected struct{}

// forwardResult carries the HTTP response from forwarding a webhook event.
type forwardResult struct {
	statusCode int
	latency    time.Duration
	eventIndex int
}

// webhookLogEntry represents a single rendered log line with optional raw JSON.
type webhookLogEntry struct {
	renderedLine string
	rawJSON      string
	forwardCode  int
	forwardTime  time.Duration
}

// WebhookViewerConfig holds initialization parameters for the viewer.
type WebhookViewerConfig struct {
	Client       *api.Client
	ForwardToURL string
	EventFilters []string
}

// WebhookViewerModel is the Bubbletea model for the live webhook viewer.
type WebhookViewerModel struct {
	config       WebhookViewerConfig
	viewport     viewport.Model
	entries      []webhookLogEntry
	eventCount   int
	connected    bool
	showJSON     bool
	cancelStream context.CancelFunc
	ready        bool
}

// NewWebhookViewer creates a new WebhookViewerModel.
func NewWebhookViewer(config WebhookViewerConfig) WebhookViewerModel {
	viewportModel := viewport.New(webhookViewerWidth, webhookViewerHeight)
	viewportModel.SetContent("")

	return WebhookViewerModel{
		config:    config,
		viewport:  viewportModel,
		entries:   make([]webhookLogEntry, 0),
		connected: false,
		showJSON:  false,
		ready:     false,
	}
}

// Init starts the SSE stream subscription.
func (model WebhookViewerModel) Init() tea.Cmd {
	return model.subscribeToStream()
}

// Update handles incoming messages and key presses.
func (model WebhookViewerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMessage := message.(type) {
	case tea.KeyMsg:
		return model.handleKeyPress(typedMessage)

	case tea.WindowSizeMsg:
		return model.handleWindowResize(typedMessage)

	case webhookEventReceived:
		return model.handleEventReceived(typedMessage)

	case webhookStreamDisconnected:
		return model.handleDisconnection()

	case forwardResult:
		return model.handleForwardResult(typedMessage)
	}

	updatedViewport, viewportCmd := model.viewport.Update(message)
	model.viewport = updatedViewport
	return model, viewportCmd
}

// View renders the complete TUI.
func (model WebhookViewerModel) View() string {
	var builder strings.Builder

	builder.WriteString(model.renderStatusBar())
	builder.WriteString("\n")
	builder.WriteString(model.viewport.View())
	builder.WriteString("\n")
	builder.WriteString(model.renderFooter())

	return builder.String()
}

func (model WebhookViewerModel) handleKeyPress(keyMessage tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMessage.String() {
	case "q", "ctrl+c":
		if model.cancelStream != nil {
			model.cancelStream()
		}
		return model, tea.Quit

	case "j":
		model.showJSON = !model.showJSON
		model.refreshViewportContent()
		return model, nil
	}

	updatedViewport, viewportCmd := model.viewport.Update(keyMessage)
	model.viewport = updatedViewport
	return model, viewportCmd
}

func (model *WebhookViewerModel) handleWindowResize(sizeMessage tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	model.viewport.Width = sizeMessage.Width
	model.viewport.Height = sizeMessage.Height - statusBarHeight
	model.ready = true
	model.refreshViewportContent()
	return model, nil
}

func (model *WebhookViewerModel) handleEventReceived(received webhookEventReceived) (tea.Model, tea.Cmd) {
	model.connected = true

	if !model.shouldIncludeEvent(received.event.Event) {
		return model, model.waitForNextEvent()
	}

	model.eventCount++

	entry := buildLogEntry(received.event)
	model.entries = append(model.entries, entry)
	model.refreshViewportContent()
	model.viewport.GotoBottom()

	var commands []tea.Cmd
	commands = append(commands, model.waitForNextEvent())

	if model.config.ForwardToURL != "" {
		commands = append(commands, model.forwardEvent(received.event, len(model.entries)-1))
	}

	return model, tea.Batch(commands...)
}

func (model *WebhookViewerModel) handleDisconnection() (tea.Model, tea.Cmd) {
	model.connected = false
	return model, model.subscribeToStream()
}

func (model *WebhookViewerModel) handleForwardResult(result forwardResult) (tea.Model, tea.Cmd) {
	if result.eventIndex >= len(model.entries) {
		return model, nil
	}

	model.entries[result.eventIndex].forwardCode = result.statusCode
	model.entries[result.eventIndex].forwardTime = result.latency
	model.refreshViewportContent()
	return model, nil
}

func (model WebhookViewerModel) shouldIncludeEvent(eventType string) bool {
	if len(model.config.EventFilters) == 0 {
		return true
	}

	for _, filter := range model.config.EventFilters {
		if filter == eventType {
			return true
		}
	}

	return false
}

func (model WebhookViewerModel) subscribeToStream() tea.Cmd {
	return func() tea.Msg {
		// Background context here is intentional: the stream lifetime is
		// owned by the bubbletea program, which is torn down via the
		// process exiting. Using context.WithCancel without storing the
		// cancelFunc on the model would leak it (gosec G118), so we pick
		// the simpler model: the underlying SSE goroutine ends when the
		// channel reader stops or the HTTP connection drops.
		streamContext := context.Background()

		eventChannel, streamError := model.config.Client.StreamEvents(streamContext)
		if streamError != nil {
			return webhookStreamDisconnected{}
		}

		event, channelOpen := <-eventChannel
		if !channelOpen {
			return webhookStreamDisconnected{}
		}

		return webhookEventReceived{event: event}
	}
}

func (model WebhookViewerModel) waitForNextEvent() tea.Cmd {
	return func() tea.Msg {
		streamContext := context.Background()

		eventChannel, streamError := model.config.Client.StreamEvents(streamContext)
		if streamError != nil {
			return webhookStreamDisconnected{}
		}

		event, channelOpen := <-eventChannel
		if !channelOpen {
			return webhookStreamDisconnected{}
		}

		return webhookEventReceived{event: event}
	}
}

func (model WebhookViewerModel) forwardEvent(event api.SSEEvent, eventIndex int) tea.Cmd {
	forwardURL := model.config.ForwardToURL

	return func() tea.Msg {
		startTime := time.Now()

		httpClient := &http.Client{
			Timeout: forwardingTimeoutSeconds * time.Second,
		}

		requestBody := bytes.NewBufferString(event.Data)
		httpRequest, requestError := http.NewRequest(http.MethodPost, forwardURL, requestBody)
		if requestError != nil {
			return forwardResult{
				statusCode: 0,
				latency:    time.Since(startTime),
				eventIndex: eventIndex,
			}
		}

		httpRequest.Header.Set("Content-Type", "application/json")

		httpResponse, responseError := httpClient.Do(httpRequest)
		if responseError != nil {
			return forwardResult{
				statusCode: 0,
				latency:    time.Since(startTime),
				eventIndex: eventIndex,
			}
		}
		defer httpResponse.Body.Close()
		_, _ = io.Copy(io.Discard, httpResponse.Body)

		return forwardResult{
			statusCode: httpResponse.StatusCode,
			latency:    time.Since(startTime),
			eventIndex: eventIndex,
		}
	}
}

func (model *WebhookViewerModel) refreshViewportContent() {
	var lines []string

	for entryIndex, entry := range model.entries {
		line := entry.renderedLine

		if entry.forwardCode > 0 {
			line += "  " + renderForwardStatus(entry.forwardCode, entry.forwardTime)
		}

		lines = append(lines, line)

		isLastEntry := entryIndex == len(model.entries)-1
		if model.showJSON && isLastEntry && entry.rawJSON != "" {
			lines = append(lines, jsonStyle.Render(entry.rawJSON))
		}
	}

	model.viewport.SetContent(strings.Join(lines, "\n"))
}

func (model WebhookViewerModel) renderStatusBar() string {
	var statusText string

	if model.connected {
		statusText = connectedStyle.Render(fmt.Sprintf("%s %s", statusIconConnected, statusLabelConnected))
	} else {
		statusText = disconnStyle.Render(fmt.Sprintf("%s %s", statusIconDisconnected, statusLabelDisconnected))
	}

	counterText := counterStyle.Render(fmt.Sprintf("%s%d", eventCounterLabel, model.eventCount))

	return fmt.Sprintf("%s    %s", statusText, counterText)
}

func (model WebhookViewerModel) renderFooter() string {
	return hintStyle.Render(fmt.Sprintf("  %s  %s", quitHint, jsonToggleHint))
}

func buildLogEntry(event api.SSEEvent) webhookLogEntry {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	phone := extractPhoneFromEventData(event.Data)

	renderedLine := fmt.Sprintf(
		"%s  %s  %s",
		timestampStyle.Render(timestamp),
		eventTypeStyle.Render(padEventType(event.Event)),
		phoneStyle.Render(phone),
	)

	prettyJSON := formatJSONData(event.Data)

	return webhookLogEntry{
		renderedLine: renderedLine,
		rawJSON:      prettyJSON,
	}
}

func renderForwardStatus(statusCode int, latency time.Duration) string {
	latencyMilliseconds := latency.Milliseconds()
	latencyText := latencyStyle.Render(fmt.Sprintf("%d%s", latencyMilliseconds, latencyUnitMilliseconds))

	if statusCode >= httpStatusServerError {
		return statusErrStyle.Render(fmt.Sprintf("[%d ERR]", statusCode)) + "  " + latencyText
	}

	if statusCode >= httpStatusClientError {
		return statusErrStyle.Render(fmt.Sprintf("[%d ERR]", statusCode)) + "  " + latencyText
	}

	if statusCode > 0 && statusCode < httpStatusOKThreshold {
		return statusOKStyle.Render(fmt.Sprintf("[%d OK]", statusCode)) + "  " + latencyText
	}

	return statusErrStyle.Render("[ERR]") + "  " + latencyText
}

func extractPhoneFromEventData(data string) string {
	var payload map[string]any
	if jsonError := json.Unmarshal([]byte(data), &payload); jsonError != nil {
		return ""
	}

	receiver := extractStringField(payload, "receiver")
	if receiver == "" {
		receiver = extractStringField(payload, "to")
	}
	if receiver == "" {
		receiver = extractStringField(payload, "phone")
	}

	return maskPhoneNumber(receiver)
}

func extractStringField(payload map[string]any, fieldName string) string {
	value, exists := payload[fieldName]
	if !exists {
		return ""
	}

	stringValue, isString := value.(string)
	if !isString {
		return ""
	}

	return stringValue
}

func maskPhoneNumber(phone string) string {
	if phone == "" {
		return ""
	}

	cleaned := strings.TrimPrefix(phone, whatsappReceiverPrefix)

	if len(cleaned) <= phoneMaskVisibleDigits {
		return cleaned
	}

	visiblePart := cleaned[len(cleaned)-phoneMaskVisibleDigits:]
	return phoneMaskPrefix + visiblePart
}

func padEventType(eventType string) string {
	const eventTypePadWidth = 22
	if len(eventType) >= eventTypePadWidth {
		return eventType
	}
	return eventType + strings.Repeat(" ", eventTypePadWidth-len(eventType))
}

func formatJSONData(data string) string {
	var parsed any
	if jsonError := json.Unmarshal([]byte(data), &parsed); jsonError != nil {
		return data
	}

	formatted, formatError := json.MarshalIndent(parsed, "    ", jsonIndentSpaces)
	if formatError != nil {
		return data
	}

	return "    " + string(formatted)
}
