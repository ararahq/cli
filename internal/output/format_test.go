package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	cases := []struct {
		input string
		want  Format
	}{
		{"json", FormatJSON},
		{" JSON ", FormatJSON},
		{"table", FormatTable},
		{"text", FormatText},
		{"", FormatText},
		{"banana", FormatText},
		{"stream-json", FormatStreamJSON},
		{"NDJSON", FormatStreamJSON},
		{"stream", FormatStreamJSON},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := ParseFormat(tc.input); got != tc.want {
				t.Errorf("ParseFormat(%q): want %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestFormat_IsMachineReadable(t *testing.T) {
	if !FormatJSON.IsMachineReadable() {
		t.Error("json should be machine readable")
	}
	if !FormatStreamJSON.IsMachineReadable() {
		t.Error("stream-json should be machine readable")
	}
	if FormatText.IsMachineReadable() {
		t.Error("text is not machine readable")
	}
	if FormatTable.IsMachineReadable() {
		t.Error("table is not machine readable")
	}
}

func TestWriteJSONLine_EmitsSingleLine(t *testing.T) {
	buffer := &bytes.Buffer{}

	if err := WriteJSONLine(buffer, map[string]any{"id": "abc", "n": 1}); err != nil {
		t.Fatalf("WriteJSONLine: %v", err)
	}
	if err := WriteJSONLine(buffer, map[string]any{"id": "def", "n": 2}); err != nil {
		t.Fatalf("WriteJSONLine: %v", err)
	}

	rawOutput := buffer.String()
	lines := strings.Split(strings.TrimRight(rawOutput, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d (raw=%q)", len(lines), rawOutput)
	}

	for index, line := range lines {
		if strings.Contains(line, "\n") {
			t.Errorf("line %d contains embedded newline", index)
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Errorf("line %d is not valid JSON: %v (raw=%q)", index, err, line)
		}
	}
}

func TestWriteJSONLine_MarshalError(t *testing.T) {
	buffer := &bytes.Buffer{}
	if err := WriteJSONLine(buffer, make(chan int)); err == nil {
		t.Error("expected marshal error on channel")
	}
	if buffer.Len() != 0 {
		t.Errorf("buffer should be untouched on marshal error, got %q", buffer.String())
	}
}

type failingWriter struct {
	successfulCalls int
	calls           int
}

func (writer *failingWriter) Write(payload []byte) (int, error) {
	writer.calls++
	if writer.calls > writer.successfulCalls {
		return 0, errSimulatedWriteFailure
	}
	return len(payload), nil
}

var errSimulatedWriteFailure = &simulatedError{}

type simulatedError struct{}

func (*simulatedError) Error() string { return "simulated write failure" }

func TestWriteJSONLine_WriteError(t *testing.T) {
	if err := WriteJSONLine(&failingWriter{successfulCalls: 0}, map[string]int{"a": 1}); err == nil {
		t.Error("expected write error on payload")
	}
	if err := WriteJSONLine(&failingWriter{successfulCalls: 1}, map[string]int{"a": 1}); err == nil {
		t.Error("expected write error on newline")
	}
}

func TestPrintJSON_MarshalError(t *testing.T) {
	if err := PrintJSON(make(chan int)); err == nil {
		t.Error("expected error marshaling channel")
	}
}

func TestPrintJSON_ValidJSONStructure(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	encoded, err := json.MarshalIndent(sample{Name: "x", Value: 7}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var decoded sample
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("MarshalIndent output is not valid JSON: %v", err)
	}
	if decoded.Name != "x" || decoded.Value != 7 {
		t.Errorf("roundtrip mismatch: %+v", decoded)
	}
}
