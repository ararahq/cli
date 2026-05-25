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
// Callers wrap the result in renderResultPanel which handles the heading,
// so this just returns the indented table block.
func renderREPLTable(_ int, headers []string, rows [][]string) string {
	tbl := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderStyle(sDim).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(replTableStyle)

	return indentTableBlock(tbl.String())
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
