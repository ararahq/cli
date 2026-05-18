package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/tui/components"
)

const (
	dashboardRefreshInterval = 30 * time.Second
	recentMessagesLimit      = 10
	firstPage                = 0
	percentMultiplier        = 100.0

	brandOrange    = "#FF6B35"
	surfaceColor   = "#1A1A2E"
	borderColor    = "#333355"
	textWhite      = "#FFFFFF"
	textDim        = "#888888"
	footerColor    = "#555555"
	statBoxWidth   = 28
	statBoxHeight  = 5
	tableMinWidth  = 80
	phoneColumnLen = 15
	timeColumnLen  = 12
)

type dashboardDataMsg struct {
	metrics       map[string]any
	walletBalance map[string]any
	messages      map[string]any
}

type dashboardErrorMsg struct {
	fetchError error
}

type tickMsg time.Time

type DashboardModel struct {
	apiClient     *api.Client
	profileName   string
	mode          string
	metrics       map[string]any
	walletBalance map[string]any
	messages      map[string]any
	loading       bool
	errorMessage  error
	width         int
	height        int
	spinner       components.Spinner
}

func NewDashboardModel(apiClient *api.Client, profileName string, mode string) DashboardModel {
	return DashboardModel{
		apiClient:   apiClient,
		profileName: profileName,
		mode:        mode,
		loading:     true,
		spinner:     components.NewSpinner(),
	}
}

func (dashboard DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		dashboard.spinner.Init(),
		dashboard.fetchDashboardData(),
		tickAfterInterval(),
	)
}

func (dashboard DashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMessage := message.(type) {
	case tea.KeyMsg:
		return dashboard.handleKeyPress(typedMessage)
	case tea.WindowSizeMsg:
		dashboard.width = typedMessage.Width
		dashboard.height = typedMessage.Height
		return dashboard, nil
	case dashboardDataMsg:
		return dashboard.handleDataReceived(typedMessage), nil
	case dashboardErrorMsg:
		dashboard.loading = false
		dashboard.errorMessage = typedMessage.fetchError
		return dashboard, nil
	case tickMsg:
		return dashboard, tea.Batch(
			dashboard.fetchDashboardData(),
			tickAfterInterval(),
		)
	default:
		updatedSpinner, spinnerCommand := dashboard.spinner.Update(message)
		dashboard.spinner = updatedSpinner
		return dashboard, spinnerCommand
	}
}

func (dashboard DashboardModel) View() string {
	if dashboard.width == 0 {
		return ""
	}

	var sections []string

	sections = append(sections, components.RenderHeader(dashboard.profileName, dashboard.mode))
	sections = append(sections, "")

	if dashboard.loading && dashboard.metrics == nil {
		sections = append(sections, dashboard.spinner.View())
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	if dashboard.errorMessage != nil && dashboard.metrics == nil {
		sections = append(sections, renderErrorPanel(dashboard.errorMessage))
		sections = append(sections, "")
		sections = append(sections, renderFooter())
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	sections = append(sections, dashboard.renderStatBoxes())
	sections = append(sections, "")
	sections = append(sections, dashboard.renderMessagesTable())
	sections = append(sections, "")
	sections = append(sections, renderFooter())

	if dashboard.loading {
		refreshIndicator := lipgloss.NewStyle().Foreground(lipgloss.Color(textDim)).Render("  refreshing...")
		sections = append(sections, refreshIndicator)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (dashboard DashboardModel) handleKeyPress(keyMessage tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMessage.String() {
	case "q", "ctrl+c":
		return dashboard, tea.Quit
	case "r":
		dashboard.loading = true
		return dashboard, dashboard.fetchDashboardData()
	default:
		return dashboard, nil
	}
}

func (dashboard DashboardModel) handleDataReceived(data dashboardDataMsg) DashboardModel {
	dashboard.loading = false
	dashboard.errorMessage = nil
	dashboard.metrics = data.metrics
	dashboard.walletBalance = data.walletBalance
	dashboard.messages = data.messages

	return dashboard
}

func (dashboard DashboardModel) fetchDashboardData() tea.Cmd {
	return func() tea.Msg {
		mode := strings.ToLower(dashboard.mode)

		metrics, metricsError := dashboard.apiClient.GetMetrics(mode)
		if metricsError != nil {
			return dashboardErrorMsg{fetchError: metricsError}
		}

		walletBalance, walletError := dashboard.apiClient.GetWalletBalance(mode)
		if walletError != nil {
			return dashboardErrorMsg{fetchError: walletError}
		}

		messages, messagesError := dashboard.apiClient.GetMessages(mode, firstPage, recentMessagesLimit)
		if messagesError != nil {
			return dashboardErrorMsg{fetchError: messagesError}
		}

		return dashboardDataMsg{
			metrics:       metrics,
			walletBalance: walletBalance,
			messages:      messages,
		}
	}
}

func tickAfterInterval() tea.Cmd {
	return tea.Tick(dashboardRefreshInterval, func(currentTime time.Time) tea.Msg {
		return tickMsg(currentTime)
	})
}

func (dashboard DashboardModel) renderStatBoxes() string {
	creditsBox := renderStatBox("Credits", dashboard.formatCredits(), brandOrange)
	messagesSentBox := renderStatBox("Messages Sent", dashboard.formatMessagesSent(), "#2196F3")
	deliveryRateBox := renderStatBox("Delivery Rate", dashboard.formatDeliveryRate(), "#00C853")

	return lipgloss.JoinHorizontal(lipgloss.Top, creditsBox, "  ", messagesSentBox, "  ", deliveryRateBox)
}

func renderStatBox(title string, value string, accentColor string) string {
	boxStyle := lipgloss.NewStyle().
		Width(statBoxWidth).
		Height(statBoxHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(1, 2).
		Background(lipgloss.Color(surfaceColor))

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textDim)).
		MarginBottom(1)

	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(accentColor))

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		valueStyle.Render(value),
	)

	return boxStyle.Render(content)
}

func (dashboard DashboardModel) formatCredits() string {
	balance := extractFloat(dashboard.walletBalance, "balance")

	return fmt.Sprintf("R$ %.2f", balance)
}

func (dashboard DashboardModel) formatMessagesSent() string {
	count := extractFloat(dashboard.metrics, "sent")

	return fmt.Sprintf("%.0f", count)
}

func (dashboard DashboardModel) formatDeliveryRate() string {
	sent := extractFloat(dashboard.metrics, "sent")
	delivered := extractFloat(dashboard.metrics, "delivered")

	if sent == 0 {
		return "0.0%"
	}

	rate := (delivered / sent) * percentMultiplier

	return fmt.Sprintf("%.1f%%", rate)
}

func (dashboard DashboardModel) renderMessagesTable() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(textWhite)).
		MarginBottom(1)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(textDim))

	output := titleStyle.Render("Recent Messages")
	output += "\n"
	output += headerStyle.Render(fmt.Sprintf("  %-14s %-*s %-20s %s", "STATUS", phoneColumnLen, "PHONE", "TEMPLATE", "TIME"))
	output += "\n"

	messageRows := dashboard.extractMessageRows()
	if len(messageRows) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(textDim))
		output += emptyStyle.Render("  No messages found")
		return output
	}

	for _, row := range messageRows {
		output += dashboard.renderMessageRow(row) + "\n"
	}

	return strings.TrimRight(output, "\n")
}

type messageRow struct {
	status       string
	receiver     string
	templateName string
	createdAt    string
}

func (dashboard DashboardModel) extractMessageRows() []messageRow {
	if dashboard.messages == nil {
		return nil
	}

	dataSlice, dataExists := dashboard.messages["messages"]
	if !dataExists {
		return nil
	}

	messageList, isList := dataSlice.([]any)
	if !isList {
		return nil
	}

	var rows []messageRow
	for _, item := range messageList {
		messageMap, isMap := item.(map[string]any)
		if !isMap {
			continue
		}

		rows = append(rows, messageRow{
			status:       extractString(messageMap, "status"),
			receiver:     extractString(messageMap, "receiver"),
			templateName: extractString(messageMap, "templateName"),
			createdAt:    extractString(messageMap, "createdAt"),
		})
	}

	return rows
}

func (dashboard DashboardModel) renderMessageRow(row messageRow) string {
	statusBadge := components.RenderStatusBadge(row.status)
	phone := truncateString(cleanPhone(row.receiver), phoneColumnLen)
	template := truncateString(row.templateName, 20)
	timeAgo := formatTimeAgo(row.createdAt)

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(textDim))

	return fmt.Sprintf("  %-14s %-*s %-20s %s",
		statusBadge,
		phoneColumnLen, phone,
		template,
		dimStyle.Render(timeAgo),
	)
}

func renderErrorPanel(fetchError error) string {
	boxStyle := lipgloss.NewStyle().
		Width(60).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF1744")).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1744"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(textDim))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD600"))

	errorText := fetchError.Error()

	content := titleStyle.Render("Could not load dashboard") + "\n\n"
	content += dimStyle.Render(errorText) + "\n\n"

	if strings.Contains(errorText, "403") {
		content += hintStyle.Render("Possible causes:") + "\n"
		content += dimStyle.Render("  - API key may not have dashboard access") + "\n"
		content += dimStyle.Render("  - Cloudflare may be blocking CLI requests") + "\n"
		content += dimStyle.Render("  - Organization may not be in ACTIVE status") + "\n\n"
		content += dimStyle.Render("Try: arara templates list  (to verify API connectivity)")
	} else if strings.Contains(errorText, "401") {
		content += hintStyle.Render("Your API key appears to be invalid or expired.") + "\n"
		content += dimStyle.Render("Run: arara login --key <your-key>")
	} else {
		content += hintStyle.Render("Check your connection and try again.") + "\n"
		content += dimStyle.Render("Press 'r' to retry.")
	}

	return boxStyle.Render(content)
}

func renderFooter() string {
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(footerColor))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(textDim))

	return footerStyle.Render(
		keyStyle.Render("q") + " quit  " +
			keyStyle.Render("r") + " refresh",
	)
}

func extractFloat(data map[string]any, key string) float64 {
	if data == nil {
		return 0
	}

	rawValue, exists := data[key]
	if !exists {
		return 0
	}

	switch typedValue := rawValue.(type) {
	case float64:
		return typedValue
	case int:
		return float64(typedValue)
	case int64:
		return float64(typedValue)
	default:
		return 0
	}
}

func extractString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}

	rawValue, exists := data[key]
	if !exists {
		return ""
	}

	stringValue, isString := rawValue.(string)
	if !isString {
		return fmt.Sprintf("%v", rawValue)
	}

	return stringValue
}

func cleanPhone(receiver string) string {
	cleaned := strings.TrimPrefix(receiver, "whatsapp:")

	return cleaned
}

func truncateString(input string, maxLength int) string {
	if len(input) <= maxLength {
		return input
	}

	const ellipsisSuffix = "..."
	truncatedLength := maxLength - len(ellipsisSuffix)
	if truncatedLength < 0 {
		return input[:maxLength]
	}

	return input[:truncatedLength] + ellipsisSuffix
}

func formatTimeAgo(isoTimestamp string) string {
	if isoTimestamp == "" {
		return ""
	}

	parsedTime, parseError := time.Parse(time.RFC3339, isoTimestamp)
	if parseError != nil {
		return isoTimestamp
	}

	elapsed := time.Since(parsedTime)

	return humanizeDuration(elapsed)
}

func humanizeDuration(duration time.Duration) string {
	const (
		secondsPerMinute = 60
		minutesPerHour   = 60
		hoursPerDay      = 24
	)

	totalSeconds := int(math.Abs(duration.Seconds()))

	if totalSeconds < secondsPerMinute {
		return "just now"
	}

	totalMinutes := totalSeconds / secondsPerMinute
	if totalMinutes < minutesPerHour {
		return fmt.Sprintf("%dm ago", totalMinutes)
	}

	totalHours := totalMinutes / minutesPerHour
	if totalHours < hoursPerDay {
		return fmt.Sprintf("%dh ago", totalHours)
	}

	totalDays := totalHours / hoursPerDay

	return fmt.Sprintf("%dd ago", totalDays)
}
