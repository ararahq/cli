package config

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func setupKeyringMock(t *testing.T) {
	t.Helper()
	keyring.MockInit()
}

func setupKeyringFailingMock(t *testing.T, cause error) {
	t.Helper()
	keyring.MockInitWithError(cause)
}

func TestStoreAPIKey_RejectsEmptyInputs(t *testing.T) {
	setupTempHome(t)
	setupKeyringMock(t)

	if err := StoreAPIKey("", "ara_test_x"); err == nil {
		t.Error("expected error on empty profile name")
	}
	if err := StoreAPIKey("default", ""); err == nil {
		t.Error("expected error on empty key")
	}
}

func TestStoreAndGetAPIKey_KeyringRoundtrip(t *testing.T) {
	setupTempHome(t)
	setupKeyringMock(t)

	if err := StoreAPIKey("default", "ara_test_secret"); err != nil {
		t.Fatalf("StoreAPIKey: %v", err)
	}

	got, err := GetAPIKey("default")
	if err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	if got != "ara_test_secret" {
		t.Errorf("expected ara_test_secret, got %q", got)
	}
}

func TestGetAPIKey_RejectsEmptyProfile(t *testing.T) {
	if _, err := GetAPIKey(""); err == nil {
		t.Error("expected error on empty profile")
	}
}

func TestDeleteAPIKey_RemovesFromKeyring(t *testing.T) {
	setupTempHome(t)
	setupKeyringMock(t)

	if err := StoreAPIKey("default", "x"); err != nil {
		t.Fatalf("StoreAPIKey: %v", err)
	}
	if err := DeleteAPIKey("default"); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	if _, err := GetAPIKey("default"); err == nil {
		t.Error("expected error fetching deleted key")
	}
}

func TestDeleteAPIKey_RejectsEmptyProfile(t *testing.T) {
	if err := DeleteAPIKey(""); err == nil {
		t.Error("expected error on empty profile")
	}
}

func TestStoreAPIKey_FallsBackToConfigFile(t *testing.T) {
	setupTempHome(t)
	setupKeyringFailingMock(t, errors.New("simulated keyring outage"))

	if err := StoreAPIKey("default", "ara_test_fallback"); err != nil {
		t.Fatalf("StoreAPIKey should fall back, got: %v", err)
	}

	got, err := GetAPIKey("default")
	if err != nil {
		t.Fatalf("GetAPIKey from fallback: %v", err)
	}
	if got != "ara_test_fallback" {
		t.Errorf("fallback roundtrip mismatch, got %q", got)
	}
}

func TestDeleteAPIKey_FallsBackToConfigFile(t *testing.T) {
	setupTempHome(t)
	setupKeyringFailingMock(t, errors.New("simulated keyring outage"))

	if err := StoreAPIKey("default", "ara_test_x"); err != nil {
		t.Fatalf("StoreAPIKey: %v", err)
	}

	if err := DeleteAPIKey("default"); err != nil {
		t.Fatalf("DeleteAPIKey fallback: %v", err)
	}

	if _, err := GetAPIKey("default"); err == nil {
		t.Error("expected GetAPIKey to fail after fallback delete")
	}
}

func TestGetAPIKey_FallbackProfileMissing(t *testing.T) {
	setupTempHome(t)
	setupKeyringFailingMock(t, errors.New("nope"))

	if _, err := GetAPIKey("ghost"); err == nil {
		t.Error("expected error when profile not in config fallback")
	}
}

func TestDeleteAPIKey_FallbackProfileMissingIsNop(t *testing.T) {
	setupTempHome(t)
	setupKeyringFailingMock(t, errors.New("nope"))

	if err := DeleteAPIKey("ghost"); err != nil {
		t.Errorf("deleting unknown profile via fallback should be nop, got: %v", err)
	}
}

func TestGetAPIKey_FallbackEmptyKey(t *testing.T) {
	setupTempHome(t)
	setupKeyringFailingMock(t, errors.New("nope"))

	configuration, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	configuration.Profiles["default"] = Profile{APIURL: DefaultAPIURL, Mode: DefaultMode}
	if err := Save(configuration); err != nil {
		t.Fatal(err)
	}

	if _, err := GetAPIKey("default"); err == nil {
		t.Error("expected error when fallback profile has no key")
	}
}
