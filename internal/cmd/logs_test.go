package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/output"
)

func TestExtractMapString_StringValue(t *testing.T) {
	data := map[string]any{"id": "msg_123"}
	if got := extractMapString(data, "id"); got != "msg_123" {
		t.Errorf("want %q, got %q", "msg_123", got)
	}
}

func TestExtractMapString_MissingKeyReturnsEmpty(t *testing.T) {
	data := map[string]any{"id": "x"}
	if got := extractMapString(data, "missing"); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestExtractMapString_NonStringValueIsFormatted(t *testing.T) {
	data := map[string]any{"count": 42}
	if got := extractMapString(data, "count"); got != "42" {
		t.Errorf("want %q, got %q", "42", got)
	}
}

func TestBuildMessageRow_AllFieldsPresent(t *testing.T) {
	message := map[string]any{
		"id":           "msg_1",
		"receiver":     "+5511999999999",
		"templateName": "welcome",
		"status":       "delivered",
		"createdAt":    "2026-05-10T12:00:00Z",
	}

	row := buildMessageRow(message)

	if len(row) != 5 {
		t.Fatalf("row length: want 5, got %d", len(row))
	}
	expected := []string{"msg_1", "+5511999999999", "welcome", "delivered", "2026-05-10T12:00:00Z"}
	for index, want := range expected {
		if row[index] != want {
			t.Errorf("row[%d]: want %q, got %q", index, want, row[index])
		}
	}
}

func TestBuildMessageRow_EmptyTemplateRendersDash(t *testing.T) {
	message := map[string]any{
		"id":       "msg_freeform",
		"receiver": "+5511111111111",
		"status":   "sent",
	}

	row := buildMessageRow(message)

	if row[2] != "-" {
		t.Errorf("missing templateName should render as '-', got %q", row[2])
	}
}

func TestMessagesTableData_FoundFalseWhenDataMissing(t *testing.T) {
	_, _, found := messagesTableData(map[string]any{})
	if found {
		t.Error("expected found=false when data key missing")
	}
}

func TestMessagesTableData_FoundFalseWhenDataEmpty(t *testing.T) {
	_, _, found := messagesTableData(map[string]any{"data": []any{}})
	if found {
		t.Error("expected found=false for empty data")
	}
}

func TestMessagesTableData_FoundFalseWhenDataWrongType(t *testing.T) {
	_, _, found := messagesTableData(map[string]any{"data": "not-a-slice"})
	if found {
		t.Error("expected found=false when data is not a slice")
	}
}

func TestMessagesTableData_BuildsHeadersAndRows(t *testing.T) {
	response := map[string]any{
		"data": []any{
			map[string]any{
				"id":           "msg_1",
				"receiver":     "+5511999999999",
				"templateName": "hello",
				"status":       "delivered",
				"createdAt":    "2026-05-10",
			},
			map[string]any{
				"id":        "msg_2",
				"receiver":  "+5511888888888",
				"status":    "queued",
				"createdAt": "2026-05-10",
			},
		},
	}

	headers, rows, found := messagesTableData(response)
	if !found {
		t.Fatal("expected found=true")
	}
	if len(headers) != 5 {
		t.Errorf("headers: want 5, got %d", len(headers))
	}
	if len(rows) != 2 {
		t.Fatalf("rows: want 2, got %d", len(rows))
	}
	if rows[1][2] != "-" {
		t.Errorf("missing template should be '-', got %q", rows[1][2])
	}
}

func TestMessagesTableData_SkipsNonMapEntries(t *testing.T) {
	response := map[string]any{
		"data": []any{
			"not-a-map",
			map[string]any{"id": "msg_1", "receiver": "+551199", "status": "sent", "createdAt": "x"},
		},
	}

	_, rows, found := messagesTableData(response)
	if !found {
		t.Fatal("expected found=true")
	}
	if len(rows) != 1 {
		t.Errorf("non-map entry should be skipped, got %d rows", len(rows))
	}
}

func TestResolveLogsMode_ExplicitFlagWins(t *testing.T) {
	originalFlag := logsModeFlag
	t.Cleanup(func() { logsModeFlag = originalFlag })

	logsModeFlag = "production"
	// nil client never read when flag is set.
	if got := resolveLogsMode(nil); got != "production" {
		t.Errorf("flag should override, got %q", got)
	}
}

func TestRunLogsFetchImpl_GoldenPath(t *testing.T) {
	body := `{
		"data":[
			{"id":"msg_1","receiver":"+5511999999999","templateName":"hello","status":"delivered","createdAt":"2026-05-10"},
			{"id":"msg_2","receiver":"+5511888888888","status":"queued","createdAt":"2026-05-10"}
		],
		"pagination":{"page":0,"size":20,"totalElements":2,"totalPages":1}
	}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runLogsFetchImpl(client, output.FormatJSON, buffer, "test", 20); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), `"msg_1"`) {
		t.Errorf("JSON output missing msg id: %q", buffer.String())
	}
}

func TestRunLogsFetchImpl_EmptyShowsHint(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusOK, `{"data":[],"pagination":{}}`)
	buffer := &bytes.Buffer{}

	if err := runLogsFetchImpl(client, output.FormatText, buffer, "test", 20); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "No messages found") {
		t.Errorf("expected hint, got: %q", buffer.String())
	}
}

func TestRunLogsFetchImpl_ServerErrorReturnsError(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusInternalServerError, `{"error":{"code":"x","message":"y"}}`)
	if err := runLogsFetchImpl(client, output.FormatText, &bytes.Buffer{}, "test", 20); err == nil {
		t.Fatal("expected error from 5xx")
	}
}

func TestRunLogs_RejectsTailAndFollowTogether(t *testing.T) {
	originalTail := logsTailFlag
	originalFollow := logsFollowFlag
	t.Cleanup(func() {
		logsTailFlag = originalTail
		logsFollowFlag = originalFollow
	})
	logsTailFlag = true
	logsFollowFlag = true

	err := runLogs(nil, nil)
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention conflict, got: %v", err)
	}
}

func TestRunLogs_FetchPathUsesInjectedClient(t *testing.T) {
	originalTail := logsTailFlag
	originalFollow := logsFollowFlag
	t.Cleanup(func() {
		logsTailFlag = originalTail
		logsFollowFlag = originalFollow
	})
	logsTailFlag = false
	logsFollowFlag = false

	body := `{"data":[{"id":"m","receiver":"+1","status":"sent","createdAt":"2026-01-01"}],"pagination":{}}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runLogs(nil, nil); err != nil {
		t.Errorf("runLogs (fetch path): %v", err)
	}
}

func TestPrintMessagesTable_EmptyDoesNotPanic(t *testing.T) {
	printMessagesTable(map[string]any{"data": []any{}})
}
