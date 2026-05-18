package output

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	tableColumnPadding   = 2
	tableSeparatorChar   = "\u2500"
	tableColumnSeparator = "  "
)

// PrintTable writes a styled table to stdout. Kept for backward compat.
// New code should prefer WriteTable so the destination is explicit and
// tests can drive a bytes.Buffer.
func PrintTable(headers []string, rows [][]string) {
	WriteTable(os.Stdout, headers, rows)
}

// WriteTable renders a styled table to the supplied writer. Header is
// brand-orange-bold, separator is gray rule, body is plain. Pads each
// column to the widest cell + a fixed gap.
func WriteTable(writer io.Writer, headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	columnWidths := computeColumnWidths(headers, rows)

	writeHeaderRow(writer, headers, columnWidths)
	writeSeparatorRow(writer, columnWidths)
	writeBodyRows(writer, rows, columnWidths)
}

func computeColumnWidths(headers []string, rows [][]string) []int {
	widths := make([]int, len(headers))

	for columnIndex, header := range headers {
		widths[columnIndex] = len(header)
	}

	for _, row := range rows {
		for columnIndex, cell := range row {
			if columnIndex >= len(widths) {
				break
			}
			if len(cell) > widths[columnIndex] {
				widths[columnIndex] = len(cell)
			}
		}
	}

	for columnIndex := range widths {
		widths[columnIndex] += tableColumnPadding
	}

	return widths
}

func writeHeaderRow(writer io.Writer, headers []string, columnWidths []int) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandColorOrange))

	formattedColumns := make([]string, len(headers))
	for columnIndex, header := range headers {
		paddedHeader := padRight(header, columnWidths[columnIndex])
		formattedColumns[columnIndex] = headerStyle.Render(paddedHeader)
	}

	fmt.Fprintln(writer, strings.Join(formattedColumns, tableColumnSeparator))
}

func writeSeparatorRow(writer io.Writer, columnWidths []int) {
	separatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGray))

	segments := make([]string, len(columnWidths))
	for columnIndex, width := range columnWidths {
		segments[columnIndex] = strings.Repeat(tableSeparatorChar, width)
	}

	separatorLine := strings.Join(segments, tableColumnSeparator)
	fmt.Fprintln(writer, separatorStyle.Render(separatorLine))
}

func writeBodyRows(writer io.Writer, rows [][]string, columnWidths []int) {
	for _, row := range rows {
		formattedColumns := make([]string, len(columnWidths))
		for columnIndex := range columnWidths {
			cellValue := ""
			if columnIndex < len(row) {
				cellValue = row[columnIndex]
			}
			formattedColumns[columnIndex] = padRight(cellValue, columnWidths[columnIndex])
		}
		fmt.Fprintln(writer, strings.Join(formattedColumns, tableColumnSeparator))
	}
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}
