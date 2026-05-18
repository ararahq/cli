package cmd

import (
	"testing"

	"github.com/ararahq/cli/internal/config"
)

func TestSelectRemainingProfile_ReturnsEmptyWhenNoProfiles(t *testing.T) {
	configuration := &config.Config{Profiles: map[string]config.Profile{}}
	if got := selectRemainingProfile(configuration); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestSelectRemainingProfile_ReturnsAnyExistingProfile(t *testing.T) {
	configuration := &config.Config{
		Profiles: map[string]config.Profile{
			"only": {APIURL: "https://api.example.com", Mode: "live"},
		},
	}
	if got := selectRemainingProfile(configuration); got != "only" {
		t.Errorf("expected 'only', got %q", got)
	}
}

func TestSelectRemainingProfile_PicksOneOfMany(t *testing.T) {
	configuration := &config.Config{
		Profiles: map[string]config.Profile{
			"a": {APIURL: "https://a.example.com"},
			"b": {APIURL: "https://b.example.com"},
		},
	}
	got := selectRemainingProfile(configuration)
	if got != "a" && got != "b" {
		t.Errorf("expected 'a' or 'b', got %q", got)
	}
}

func TestRemoveProfileFromConfig_DeletesAndRotatesActive(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {},
			"prod":    {},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := removeProfileFromConfig("default"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	reloaded, _ := config.Load()
	if _, exists := reloaded.Profiles["default"]; exists {
		t.Error("default should be removed")
	}
	if reloaded.CurrentProfile != "prod" {
		t.Errorf("active should rotate to prod, got %q", reloaded.CurrentProfile)
	}
}

func TestRunLogout_RemovesActiveProfile(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles:       map[string]config.Profile{"default": {}, "prod": {}},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := runLogout(nil, nil); err != nil {
		t.Fatalf("runLogout: %v", err)
	}

	reloaded, _ := config.Load()
	if _, exists := reloaded.Profiles["default"]; exists {
		t.Error("default should be removed")
	}
}

// withFakeHome lives in profile_test.go; included here implicitly via package.
// Reference avoids unused-import warning if we add helpers later.
var _ = config.DefaultAPIURL
