package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderResultPanel wraps every async command's output in a consistent
// titled card so the eye has a clear receipt for what just ran. The header
// reads `▸ Templates · 25 results · 124ms` and the body indents the
// content under a left-side accent bar so multiple stacked results are
// scannable. Width adapts to the current terminal so the panel doesn't
// overflow into raw-line wrapping that would break tables.
func renderResultPanel(title, command string, duration time.Duration, body string, terminalWidth int) string {
	if title == "" {
		return body
	}

	// The accent bar lives in the border itself (LEFT border only) so the
	// content stays flush-left under the title without extra padding hops.
	width := panelWidthFor(terminalWidth)

	heading := buildPanelHeading(title, command, duration)
	bodyTrimmed := strings.TrimRight(body, "\n")

	// Compose heading + body so the body sits one blank line under the
	// title — much easier to read than a flush-against block.
	composed := heading + "\n" + bodyTrimmed

	panel := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(cBrand)).
		PaddingLeft(2).
		PaddingTop(1).
		PaddingBottom(1).
		Width(width).
		Render(composed)

	return "\n" + panel + "\n"
}

// renderResultErrorPanel mirrors renderResultPanel but switches the accent bar
// to red and prefixes the body with the standard ✘ glyph. Kept distinct
// (instead of overloading renderResultPanel with an err arg) because the
// error path also drops the title — there's no meaningful "row count" to
// show when the command blew up.
func renderResultErrorPanel(command string, duration time.Duration, message string, terminalWidth int) string {
	width := panelWidthFor(terminalWidth)

	heading := sError.Render("✘ failed")
	if command != "" {
		heading += sDim.Render("  ") + sMuted.Render(command)
	}
	if duration > 0 {
		heading += sDim.Render("  ·  ") + sDim.Render(formatElapsed(duration))
	}

	body := sError.Render(message)
	composed := heading + "\n" + body

	panel := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(cRed)).
		PaddingLeft(2).
		PaddingTop(1).
		PaddingBottom(1).
		Width(width).
		Render(composed)

	return "\n" + panel + "\n"
}

// buildPanelHeading assembles the breadcrumb-style title line that sits
// at the top of every result panel. Pieces are joined with subtle dot
// separators so the line scans as a single sentence rather than a tag soup.
//
// We deliberately suppress the verb when the title already starts with it
// ("Templates · 25" already says everything `templates · …` would add) —
// otherwise the eye reads the same word twice and the header gets noisy.
func buildPanelHeading(title, command string, duration time.Duration) string {
	parts := []string{sBrand.Render("▸ ") + sBold.Render(title)}
	titleLower := strings.ToLower(title)
	cmdLower := strings.ToLower(strings.TrimSpace(command))
	if cmdLower != "" && !strings.HasPrefix(titleLower, cmdLower) {
		parts = append(parts, sMuted.Render(command))
	}
	if duration > 0 {
		parts = append(parts, sDim.Render(formatElapsed(duration)))
	}
	return strings.Join(parts, sDim.Render(" · "))
}

// panelWidthFor clamps the panel width so it never exceeds the terminal
// but also doesn't shrink uselessly small. The -4 accounts for the
// left-padding + border the panel adds; the floor of 60 stops it from
// degenerating into a column when the user has a narrow split.
func panelWidthFor(terminalWidth int) int {
	if terminalWidth <= 0 {
		return panelDefaultWidth
	}
	w := terminalWidth - 4
	if w < panelMinWidth {
		return panelMinWidth
	}
	if w > panelMaxWidth {
		return panelMaxWidth
	}
	return w
}

const (
	panelDefaultWidth = 100
	panelMinWidth     = 60
	panelMaxWidth     = 140
)

// formatElapsed renders a time.Duration the way humans read perf numbers:
// "47ms" / "1.2s" / "3m" — no leading zeros, no nanoseconds, no padding.
// Used by the result panel header AND the live spinner once a command
// crosses the visibility threshold.
func formatElapsed(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1ms"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}
