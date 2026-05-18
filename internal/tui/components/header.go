package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

const (
	brandColorOrange = "#FF6B35"
	headerSeparator  = " · "
)

var (
	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(brandColorOrange))

	profileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	liveModeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00C853")).
			Background(lipgloss.Color("#1B5E20")).
			Padding(0, 1)

	testModeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFD600")).
			Background(lipgloss.Color("#4A3800")).
			Padding(0, 1)
)

func RenderHeader(profileName string, mode string) string {
	brandText := brandStyle.Render("arara")
	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(headerSeparator)
	profileText := profileStyle.Render(profileName)
	modeText := renderModeBadge(mode)

	return fmt.Sprintf("%s%s%s%s%s", brandText, separator, profileText, separator, modeText)
}

func renderModeBadge(mode string) string {
	if mode == "LIVE" {
		return liveModeStyle.Render(mode)
	}

	return testModeStyle.Render(mode)
}

func RenderModeBadge(mode string) string {
	return renderModeBadge(mode)
}
