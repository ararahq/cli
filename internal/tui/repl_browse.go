package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// browsableResult is what a list-style command produces when its rows can
// be drilled into. The REPL keeps a stack of these so the user can:
//   1. Run `templates`           → table appears, cursor lands on row 0
//   2. ↓ ↓ ↓                     → cursor moves through the table
//   3. Enter                     → detail of the selected row replaces table
//   4. Esc / q                   → table returns, cursor preserved
//
// Each row carries an opaque ID (the field the detail fetcher consumes)
// alongside its pre-styled cell strings. detailFunc receives that ID and
// returns the body of the detail view, leaving HTTP / formatting choices
// up to the caller.
type browsableResult struct {
	title      string
	command    string
	columns    []string
	rows       []browsableRow
	detailFunc func(rowID string) (string, error)
}

type browsableRow struct {
	id    string
	cells []string
}

// drillState tracks the user's position inside a browsableResult: which
// row they're on and whether they're currently looking at the detail
// view. Lives on the REPLModel; nil when no browsable result is in scope.
type drillState struct {
	result      browsableResult
	cursor      int
	viewingItem bool
	detailBody  string
	detailErr   string
}

// renderBrowsableTable lays out the table with the current cursor row
// highlighted. The cursor uses the same teal accent the rest of the REPL
// uses for "active", giving the eye a single visual language for focus.
//
// We rebuild the table on every render (cheap — at most a few hundred
// rows) so the cursor stripe stays in sync with the model without
// caching tricks.
func renderBrowsableTable(state drillState) string {
	if len(state.result.rows) == 0 {
		return sDim.Render("  (no rows)")
	}

	// Lipgloss/table doesn't let us paint a single arbitrary row, so we
	// render the rows ourselves into a slice of styled cells, then hand
	// the slice to the same renderREPLTable used elsewhere — gets us
	// zebra + ANSI-aware alignment for free.
	styledRows := make([][]string, 0, len(state.result.rows))
	for index, row := range state.result.rows {
		cells := make([]string, len(row.cells))
		copy(cells, row.cells)
		if index == state.cursor {
			// Decorate the first cell with a cursor caret so the focus
			// is visible even on terminals that swallow background colour
			// (older ssh sessions, screen readers). The caret replaces
			// the leading "• " bullet to keep column width unchanged.
			if len(cells) > 0 {
				cells[0] = decorateCursorCell(cells[0])
			}
		}
		styledRows = append(styledRows, cells)
	}

	return renderREPLTable("", len(state.result.rows), state.result.columns, styledRows)
}

// decorateCursorCell replaces the standard "• " left-anchor with a teal
// ▸ caret so the focused row gets a sharper visual hook than just a
// background highlight. The fallback (cell didn't start with the
// expected bullet) just prepends the caret + space, keeping width
// roughly stable.
func decorateCursorCell(cell string) string {
	caret := sBrand.Render("▸ ")
	// "• " encoded raw is 4 bytes (the bullet is 3 + space). Cells in
	// this codebase always start with that pattern when they were
	// rendered via the table helpers — we substitute in-place to keep
	// the cell width identical so the table doesn't re-flow.
	const bulletPrefix = "• "
	if idx := strings.Index(cell, bulletPrefix); idx >= 0 && idx < 16 {
		return cell[:idx] + caret + cell[idx+len(bulletPrefix):]
	}
	return caret + cell
}

// renderDrillFooter is the hint line that sits under a browsable result
// telling the user which keys do what. Stripe / k9s use the same idiom;
// keeps discovery embedded in the UI without forcing /help.
func renderDrillFooter(state drillState) string {
	if state.viewingItem {
		return "  " + sDim.Render("↑↓ ") + sMuted.Render("scroll") +
			sDim.Render("   esc ") + sMuted.Render("back to list") +
			sDim.Render("   q ") + sMuted.Render("close")
	}
	return "  " + sDim.Render("↑↓ ") + sMuted.Render("navigate") +
		sDim.Render("   enter ") + sMuted.Render("open") +
		sDim.Render("   esc ") + sMuted.Render("close")
}

// renderDrillDetail wraps a fetched detail body in a sub-panel so it sits
// visually one layer below the list it came from. Keeps the same accent
// bar as renderResultPanel but uses a muted heading so the eye reads it
// as "detail of the item above" rather than a fresh result.
func renderDrillDetail(state drillState, terminalWidth int) string {
	if !state.viewingItem {
		return ""
	}
	width := panelWidthFor(terminalWidth)

	heading := sBrand.Render("▾ ") + sBold.Render("Detail")
	if state.result.command != "" {
		heading += sDim.Render("  ·  ") + sMuted.Render(state.result.command)
	}

	body := state.detailBody
	if state.detailErr != "" {
		body = sError.Render("✘ " + state.detailErr)
	}

	composed := heading + "\n" + body
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(cBrandHi)).
		PaddingLeft(2).
		PaddingTop(1).
		PaddingBottom(1).
		Width(width).
		Render(composed)
}
