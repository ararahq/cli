package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

func TestPrintAPIKeysTable_RendersHeadersAndRows(t *testing.T) {
	keys := []api.APIKeyInfo{
		{Prefix: "ara_live_", LastFour: "abcd", Mode: "LIVE", CreatedAt: "2026-01-01", LastUsedAt: "2026-04-01"},
		{Prefix: "ara_test_", LastFour: "efgh", Mode: "TEST", CreatedAt: "2026-02-01", LastUsedAt: ""},
	}
	buffer := &bytes.Buffer{}

	printAPIKeysTable(buffer, keys)

	rendered := buffer.String()
	for _, expected := range []string{"PREFIX", "LAST 4", "MODE", "CREATED", "LAST USED", "ara_live_", "ara_test_", "abcd", "efgh", "LIVE", "TEST", "never"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("table should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestPrintAPIKeysTable_EmptyList(t *testing.T) {
	buffer := &bytes.Buffer{}
	printAPIKeysTable(buffer, nil)

	if !strings.Contains(buffer.String(), "PREFIX") {
		t.Errorf("empty table should still print headers, got:\n%s", buffer.String())
	}
}

func TestPrintCreatedKey_ShowsModeKeyAndWarning(t *testing.T) {
	generated := &api.GeneratedAPIKey{PlainTextKey: "ara_live_visible_only_now_xyz"}
	buffer := &bytes.Buffer{}

	printCreatedKey(buffer, generated, "LIVE")

	rendered := buffer.String()
	if !strings.Contains(rendered, "ara_live_visible_only_now_xyz") {
		t.Errorf("plaintext key must appear in output, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Mode:") {
		t.Errorf("output should label Mode, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "LIVE") {
		t.Errorf("output should include the mode value, got:\n%s", rendered)
	}
}

func TestValidateKeyMode(t *testing.T) {
	if err := validateKeyMode("LIVE", "LIVE"); err != nil {
		t.Errorf("LIVE should be valid: %v", err)
	}
	if err := validateKeyMode("TEST", "TEST"); err != nil {
		t.Errorf("TEST should be valid: %v", err)
	}
	err := validateKeyMode("STAGING", "staging")
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("error should mention original input, got: %v", err)
	}
}

func TestRunKeysListImpl_GoldenPath(t *testing.T) {
	body := `[
		{"prefix":"ara_live_","lastFour":"abcd","mode":"LIVE","createdAt":"2026-01-01","lastUsedAt":"2026-04-01"},
		{"prefix":"ara_test_","lastFour":"efgh","mode":"TEST","createdAt":"2026-02-01","lastUsedAt":""}
	]`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runKeysListImpl(client, output.FormatText, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"ara_live_", "ara_test_", "abcd", "efgh", "never"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunKeysListImpl_EmptyShowsHint(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusOK, `[]`)
	buffer := &bytes.Buffer{}

	if err := runKeysListImpl(client, output.FormatText, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "No API keys found") {
		t.Errorf("expected hint, got: %q", buffer.String())
	}
}

func TestRunKeysCreateImpl_GoldenPathRendersKey(t *testing.T) {
	body := `{"plainTextKey":"ara_live_brandnewfreshkeyxyz","prefix":"ara_live_","lastFour":"e xyz"}`
	_, client := fakeAPIServerJSON(t, http.StatusCreated, body)
	buffer := &bytes.Buffer{}

	if err := runKeysCreateImpl(client, output.FormatText, buffer, "LIVE", "test-key"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "ara_live_brandnewfreshkeyxyz") {
		t.Errorf("plaintext key should appear, got: %q", buffer.String())
	}
}

func TestRunKeysCreateImpl_JSONOutput(t *testing.T) {
	body := `{"plainTextKey":"ara_test_x","prefix":"ara_test_","lastFour":"x"}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runKeysCreateImpl(client, output.FormatJSON, buffer, "TEST", "test-key"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), `"plainTextKey"`) {
		t.Errorf("JSON output missing field: %q", buffer.String())
	}
}

func TestRunKeysList_WrapperUsesInjectedClient(t *testing.T) {
	body := `[{"prefix":"ara_live_","lastFour":"abcd","mode":"LIVE","createdAt":"2026-01-01"}]`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runKeysList(nil, nil); err != nil {
		t.Errorf("runKeysList: %v", err)
	}
}

func TestRunKeysCreate_RejectsUnknownMode(t *testing.T) {
	originalMode := keysCreateModeFlag
	t.Cleanup(func() { keysCreateModeFlag = originalMode })
	keysCreateModeFlag = "STAGING"

	err := runKeysCreate(nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "STAGING") {
		t.Errorf("error should mention rejected value, got: %v", err)
	}
}
