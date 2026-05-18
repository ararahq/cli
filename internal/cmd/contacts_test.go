package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

func TestParseContactsImport_Valid(t *testing.T) {
	raw := []byte(`[{"name":"alice","phone":"+5511999999999"},{"name":"bob","phone":"+5511888888888"}]`)

	contacts, err := parseContactsImport(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(contacts))
	}
	if contacts[0].Name != "alice" || contacts[0].Phone != "+5511999999999" {
		t.Errorf("contact[0]: %+v", contacts[0])
	}
}

func TestParseContactsImport_InvalidJSON(t *testing.T) {
	_, err := parseContactsImport([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parse, got: %v", err)
	}
}

func TestParseContactsImport_EmptyArray(t *testing.T) {
	_, err := parseContactsImport([]byte(`[]`))
	if err == nil {
		t.Fatal("expected error on empty array")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestContactsTableData_BuildsRowsAndFooter(t *testing.T) {
	response := &api.ContactsListResponse{
		Contacts: []api.ContactResponse{
			{Name: "Alice", Phone: "+5511999999999", Email: "alice@example.com", CreatedAt: "2026-01-15T10:00:00Z"},
			{Name: "Bob", Phone: "+5511888888888", Email: "", CreatedAt: "2026-02-20"},
		},
		Page:       0,
		TotalPages: 3,
		Total:      45,
	}

	headers, rows, footer := contactsTableData(response)

	if len(headers) != 4 {
		t.Errorf("headers: want 4, got %d", len(headers))
	}
	if len(rows) != 2 {
		t.Fatalf("rows: want 2, got %d", len(rows))
	}
	if rows[0][3] != "2026-01-15" {
		t.Errorf("createdAt should be truncated to date, got %q", rows[0][3])
	}
	if rows[1][2] != "-" {
		t.Errorf("missing email should render as '-', got %q", rows[1][2])
	}
	if !strings.Contains(footer, "Page 1 of 3") {
		t.Errorf("footer should be 1-indexed, got %q", footer)
	}
	if !strings.Contains(footer, "total: 45") {
		t.Errorf("footer should include total, got %q", footer)
	}
}

func TestPrintContactStats_RendersAllFields(t *testing.T) {
	stats := map[string]int64{
		"total":    100,
		"active":   85,
		"optedOut": 15,
	}
	buffer := &bytes.Buffer{}

	printContactStats(buffer, stats)

	rendered := buffer.String()
	for _, expected := range []string{"Total:", "100", "Active:", "85", "Opted-out:", "15"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestPrintContactsTable_PrintsFooterToWriter(t *testing.T) {
	response := &api.ContactsListResponse{
		Contacts:   nil,
		Page:       2,
		TotalPages: 5,
		Total:      120,
	}
	buffer := &bytes.Buffer{}

	printContactsTable(buffer, response)

	if !strings.Contains(buffer.String(), "Page 3 of 5") {
		t.Errorf("footer not in writer output, got:\n%s", buffer.String())
	}
}

func TestRunContactsListImpl_GoldenPath(t *testing.T) {
	body := `{
		"contacts":[
			{"id":"c1","name":"Alice","phone":"+5511999999999","email":"a@example.com","createdAt":"2026-01-15T10:00:00Z"}
		],
		"total":1,"page":0,"size":20,"totalPages":1
	}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runContactsListImpl(client, output.FormatText, buffer, "", 0, 20); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"Alice", "+5511999999999", "Page 1 of 1"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunContactsListImpl_EmptyShowsHint(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusOK, `{"contacts":[],"total":0,"page":0,"size":20,"totalPages":0}`)
	buffer := &bytes.Buffer{}

	if err := runContactsListImpl(client, output.FormatText, buffer, "", 0, 20); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "No contacts found") {
		t.Errorf("expected hint, got: %q", buffer.String())
	}
}

func TestRunContactsListImpl_JSONOutput(t *testing.T) {
	body := `{"contacts":[{"id":"c1","name":"x","phone":"+551199","email":""}],"total":1,"page":0,"size":20,"totalPages":1}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runContactsListImpl(client, output.FormatJSON, buffer, "", 0, 20); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), `"contacts"`) {
		t.Errorf("JSON output missing contacts field: %q", buffer.String())
	}
}

func TestRunContactsStatsImpl_GoldenPath(t *testing.T) {
	body := `{"total":150,"active":120,"optedOut":30}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runContactsStatsImpl(client, output.FormatText, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"Total:", "150", "Active:", "120", "Opted-out:", "30"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunContactsStatsImpl_ServerErrorReturnsError(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusInternalServerError, `{"error":{"code":"x","message":"y"}}`)
	if err := runContactsStatsImpl(client, output.FormatText, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from 5xx")
	}
}

func TestRunContactsImportImpl_GoldenPath(t *testing.T) {
	body := `{"importId":"imp_xyz"}`
	_, client := fakeAPIServerJSON(t, http.StatusAccepted, body)
	buffer := &bytes.Buffer{}

	contacts := []api.ContactRequest{
		{Name: "Alice", Phone: "+5511999999999"},
		{Name: "Bob", Phone: "+5511888888888"},
	}

	if err := runContactsImportImpl(client, output.FormatText, buffer, contacts); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "Imported 2 contacts") {
		t.Errorf("expected confirmation, got: %q", buffer.String())
	}
	if !strings.Contains(buffer.String(), "imp_xyz") {
		t.Errorf("expected import ID, got: %q", buffer.String())
	}
}

func TestRunContactsImportImpl_JSONOutput(t *testing.T) {
	body := `{"importId":"imp_abc"}`
	_, client := fakeAPIServerJSON(t, http.StatusAccepted, body)
	buffer := &bytes.Buffer{}

	contacts := []api.ContactRequest{{Name: "x", Phone: "+1"}}
	if err := runContactsImportImpl(client, output.FormatJSON, buffer, contacts); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), `"importId"`) || !strings.Contains(buffer.String(), `"count"`) {
		t.Errorf("JSON output missing fields: %q", buffer.String())
	}
}

func TestRunContactsList_WrapperUsesInjectedClient(t *testing.T) {
	body := `{"contacts":[{"id":"c1","name":"x","phone":"+551199","email":""}],"total":1,"page":0,"size":20,"totalPages":1}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runContactsList(nil, nil); err != nil {
		t.Errorf("runContactsList: %v", err)
	}
}

func TestRunContactsStats_WrapperUsesInjectedClient(t *testing.T) {
	body := `{"total":10,"active":8,"optedOut":2}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runContactsStats(nil, nil); err != nil {
		t.Errorf("runContactsStats: %v", err)
	}
}

func TestRunContactsSearch_RequiresQuery(t *testing.T) {
	originalQuery := contactsSearchQuery
	t.Cleanup(func() { contactsSearchQuery = originalQuery })
	contactsSearchQuery = ""

	err := runContactsSearch(nil, nil)
	if err == nil {
		t.Fatal("expected error when --query missing")
	}
}

func TestRunContactsImport_RequiresFile(t *testing.T) {
	originalFile := contactsImportFile
	t.Cleanup(func() { contactsImportFile = originalFile })
	contactsImportFile = ""

	err := runContactsImport(nil, nil)
	if err == nil {
		t.Fatal("expected error when --file missing")
	}
}
