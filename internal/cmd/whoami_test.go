package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildWhoamiInfo_LivePrefixDetectsLiveMode(t *testing.T) {
	info := buildWhoamiInfo("prod", "https://api.example.com", "ara_live_abcdefgh1234")

	if info.Profile != "prod" {
		t.Errorf("profile mismatch: %q", info.Profile)
	}
	if info.Mode != "LIVE" {
		t.Errorf("expected LIVE mode for ara_live_ key, got %q", info.Mode)
	}
	if info.APIURL != "https://api.example.com" {
		t.Errorf("api url mismatch: %q", info.APIURL)
	}
	if info.APIKey == "ara_live_abcdefgh1234" {
		t.Errorf("api key must be masked, got plaintext: %q", info.APIKey)
	}
	if !strings.HasPrefix(info.APIKey, "ara_live_") {
		t.Errorf("masked key should keep prefix, got %q", info.APIKey)
	}
}

func TestBuildWhoamiInfo_TestPrefixDetectsTestMode(t *testing.T) {
	info := buildWhoamiInfo("sandbox", "https://api.test.com", "ara_test_abcdefgh5678")

	if info.Mode != "TEST" {
		t.Errorf("expected TEST mode, got %q", info.Mode)
	}
}

func TestPrintWhoamiText_FormatsAllFields(t *testing.T) {
	info := whoamiInfo{
		Profile: "default",
		Mode:    "LIVE",
		APIURL:  "https://api.ararahq.com",
		APIKey:  "ara_live_****1234",
	}
	buffer := &bytes.Buffer{}

	printWhoamiText(buffer, info)

	rendered := buffer.String()
	for _, expected := range []string{"Profile:", "default", "Mode:", "LIVE", "API URL:", "https://api.ararahq.com", "API Key:", "****1234"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}
