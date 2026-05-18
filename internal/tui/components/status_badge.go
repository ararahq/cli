package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	statusDelivered      = "DELIVERED"
	statusRead           = "READ"
	statusPending        = "PENDING"
	statusFailed         = "FAILED"
	statusSentToProvider = "SENT_TO_PROVIDER"

	iconDelivered      = "✓"
	iconRead           = "✓✓"
	iconPending        = "○"
	iconFailed         = "✗"
	iconSentToProvider = "→"

	colorGreen  = "#00C853"
	colorBlue   = "#2196F3"
	colorYellow = "#FFD600"
	colorRed    = "#FF1744"
	colorDim    = "#888888"
)

var (
	deliveredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	readStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorBlue))
	pendingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorYellow))
	failedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Bold(true)
	sentStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDim))
)

func RenderStatusBadge(status string) string {
	normalizedStatus := strings.ToUpper(strings.TrimSpace(status))

	switch normalizedStatus {
	case statusDelivered:
		return deliveredStyle.Render(iconDelivered + " DELIVERED")
	case statusRead:
		return readStyle.Render(iconRead + " READ")
	case statusPending:
		return pendingStyle.Render(iconPending + " PENDING")
	case statusFailed:
		return failedStyle.Render(iconFailed + " FAILED")
	case statusSentToProvider:
		return sentStyle.Render(iconSentToProvider + " SENT")
	default:
		return sentStyle.Render("? " + normalizedStatus)
	}
}
