package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Format string

const (
	FormatText       Format = "text"
	FormatJSON       Format = "json"
	FormatTable      Format = "table"
	FormatStreamJSON Format = "stream-json"
)

const (
	jsonIndentPrefix = ""
	jsonIndentValue  = "  "
)

func ParseFormat(formatString string) Format {
	normalized := strings.ToLower(strings.TrimSpace(formatString))

	switch normalized {
	case string(FormatJSON):
		return FormatJSON
	case string(FormatTable):
		return FormatTable
	case string(FormatStreamJSON), "ndjson", "stream":
		return FormatStreamJSON
	default:
		return FormatText
	}
}

func (format Format) IsMachineReadable() bool {
	return format == FormatJSON || format == FormatStreamJSON
}

func PrintJSON(data any) error {
	encoded, encodeError := json.MarshalIndent(data, jsonIndentPrefix, jsonIndentValue)
	if encodeError != nil {
		return fmt.Errorf("failed to marshal data to JSON: %w", encodeError)
	}

	fmt.Fprintln(os.Stdout, string(encoded))
	return nil
}

func WriteJSONLine(writer io.Writer, data any) error {
	encoded, encodeError := json.Marshal(data)
	if encodeError != nil {
		return fmt.Errorf("failed to marshal NDJSON record: %w", encodeError)
	}

	if _, writeError := writer.Write(encoded); writeError != nil {
		return fmt.Errorf("failed to write NDJSON record: %w", writeError)
	}
	if _, writeError := writer.Write([]byte("\n")); writeError != nil {
		return fmt.Errorf("failed to write NDJSON newline: %w", writeError)
	}

	if flusher, canFlush := writer.(interface{ Sync() error }); canFlush {
		_ = flusher.Sync()
	}

	return nil
}

func PrintJSONLine(data any) error {
	return WriteJSONLine(os.Stdout, data)
}

func PrintSuccess(message string) {
	symbol := SuccessStyle.Render("\u2714")
	fmt.Fprintf(os.Stdout, "%s %s\n", symbol, message)
}

func PrintError(message string) {
	symbol := ErrorStyle.Render("\u2718")
	fmt.Fprintf(os.Stderr, "%s %s\n", symbol, message)
}

func PrintWarning(message string) {
	symbol := WarningStyle.Render("!")
	fmt.Fprintf(os.Stderr, "%s %s\n", symbol, message)
}

func PrintInfo(message string) {
	symbol := DimStyle.Render("i")
	fmt.Fprintf(os.Stdout, "%s %s\n", symbol, message)
}
