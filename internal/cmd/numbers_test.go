package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

func TestNumbersTableData_ShapeAndOrder(t *testing.T) {
	numbers := []api.PhoneNumberResponse{
		{PhoneNumber: "+5511999990000", Name: "Sales", ID: "pn_1"},
		{PhoneNumber: "+5511888880000", Name: "Support", ID: "pn_2"},
	}

	headers, rows := numbersTableData(numbers)

	if got := len(headers); got != 3 {
		t.Fatalf("headers length: want 3, got %d", got)
	}
	expectedHeaders := []string{"NUMBER", "NAME", "ID"}
	for index, expected := range expectedHeaders {
		if headers[index] != expected {
			t.Errorf("header[%d]: want %q, got %q", index, expected, headers[index])
		}
	}

	if got := len(rows); got != 2 {
		t.Fatalf("rows length: want 2, got %d", got)
	}
	if rows[0][0] != "+5511999990000" || rows[0][1] != "Sales" || rows[0][2] != "pn_1" {
		t.Errorf("row 0 mismatch: %v", rows[0])
	}
	if rows[1][0] != "+5511888880000" || rows[1][1] != "Support" || rows[1][2] != "pn_2" {
		t.Errorf("row 1 mismatch: %v", rows[1])
	}
}

func TestNumbersTableData_EmptyList(t *testing.T) {
	headers, rows := numbersTableData(nil)
	if len(headers) != 3 {
		t.Errorf("headers should still be present when list empty, got %d", len(headers))
	}
	if len(rows) != 0 {
		t.Errorf("rows should be empty, got %d", len(rows))
	}
}

func TestRunNumbersImpl_GoldenPathRendersJSON(t *testing.T) {
	body := `[{"id":"pn_1","phoneNumber":"+5511999990000","name":"Sales"}]`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runNumbersImpl(client, output.FormatJSON, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "+5511999990000") {
		t.Errorf("expected number in JSON output, got: %q", buffer.String())
	}
}

func TestRunNumbersImpl_EmptyShowsHint(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusOK, `[]`)
	buffer := &bytes.Buffer{}

	if err := runNumbersImpl(client, output.FormatText, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "No phone numbers found") {
		t.Errorf("expected hint, got: %q", buffer.String())
	}
}

func TestRunNumbersImpl_ServerErrorIsSurfaced(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusInternalServerError, `{"error":{"code":"server_error","message":"oops"}}`)
	if err := runNumbersImpl(client, output.FormatText, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from 5xx")
	}
}

// silence unused-import linting if api is not directly referenced in tests.
var _ = api.NormalizeReceiver

func TestRunNumbers_WrapperUsesInjectedClient(t *testing.T) {
	body := `[{"id":"pn_1","phoneNumber":"+5511999990000","name":"Sales"}]`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runNumbers(nil, nil); err != nil {
		t.Errorf("runNumbers: %v", err)
	}
}

func TestPrintNumbersTable_DoesNotPanic(t *testing.T) {
	printNumbersTable([]api.PhoneNumberResponse{
		{ID: "pn_1", PhoneNumber: "+5511999990000", Name: "Sales"},
	})
}

func TestWriteNumbersTable_RendersToWriter(t *testing.T) {
	buffer := &bytes.Buffer{}
	writeNumbersTable(buffer, []api.PhoneNumberResponse{
		{ID: "pn_1", PhoneNumber: "+5511999990000", Name: "Sales"},
	})
	if !strings.Contains(buffer.String(), "+5511999990000") {
		t.Errorf("expected number in output, got: %q", buffer.String())
	}
}
