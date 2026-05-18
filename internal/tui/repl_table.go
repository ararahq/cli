package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// renderREPLTable centralizes the look of every tabular output the REPL
// emits. Built on lipgloss/table because it understands ANSI escape
// sequences when computing column widths — the previous fmt.Sprintf with
// `%-Nds` padding miscounted styled cells and produced misaligned columns.
//
// title is rendered above the table as `Title (N)` for backwards-compat
// with the callers that still want a small heading inline. Most callers
// now wrap the result in renderResultPanel and pass an empty title here
// to skip the inline heading entirely.
func renderREPLTable(title string, count int, headers []string, rows [][]string) string {
	tbl := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderStyle(sDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(replTableStyle)

	rendered := indentTableBlock(tbl.String())

	if title == "" {
		return rendered
	}
	heading := "  " + sBold.Render(title) + sDim.Render(" · ") + sMuted.Render(plural(count, "row"))
	return heading + "\n\n" + rendered
}

// replTableStyle is the lipgloss StyleFunc shared by every REPL table.
//
//   - Header row: muted, slightly tracked-out via right padding so the
//     column titles read as labels not data.
//   - Body rows: zebra-stripe via background colour on even rows so a
//     dense list of 25 templates doesn't blur into a single grey wall.
//     The stripe colour is a barely-perceptible step away from the
//     terminal default; in true-color terminals it gives the table real
//     rhythm without competing with the cell content.
func replTableStyle(row, _ int) lipgloss.Style {
	if row == table.HeaderRow {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(cMuted)).
			Bold(true).
			Padding(0, 2, 0, 0)
	}
	base := lipgloss.NewStyle().Padding(0, 2, 0, 0)
	// Zebra: every other body row gets the bg-elevated tone from .pen
	// (#191f1f). Subtle against the default terminal black background
	// but distinct enough to act as a grid line — the previous attempt
	// used bg-overlay (#242e2f) which read as "did the column shift?"
	// on most terminals.
	if row%2 == 1 {
		base = base.Background(lipgloss.Color(cSurface))
	}
	return base
}

// indentTableBlock left-pads every line in a multi-line block with two
// spaces so it lines up with the rest of the REPL output.
func indentTableBlock(rendered string) string {
	lines := strings.Split(rendered, "\n")
	for index, line := range lines {
		lines[index] = "  " + line
	}
	return strings.Join(lines, "\n")
}

// plural returns "1 row" / "25 rows" — used in panel titles and table
// headings so the count line reads naturally instead of "Templates (25)".
func plural(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	return itoa(n) + " " + singular + "s"
}

// itoa is a zero-alloc int-to-string used by plural() so the hot path
// (every result panel renders one) doesn't hit fmt.Sprintf just to
// stringify a small integer.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var buf [20]byte
	idx := len(buf)
	for n > 0 {
		idx--
		buf[idx] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[idx:])
}
