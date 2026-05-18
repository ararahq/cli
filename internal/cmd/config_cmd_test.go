package cmd

import (
	"testing"

	"github.com/ararahq/cli/internal/config"
)

func newTestConfig(currentProfile string, output string) *config.Config {
	return &config.Config{
		CurrentProfile: currentProfile,
		Output:         output,
		Profiles:       map[string]config.Profile{},
	}
}

func TestApplyConfigValue_Profile(t *testing.T) {
	cfg := newTestConfig("default", "text")
	applyConfigValue(cfg, configKeyProfile, "production")

	if cfg.CurrentProfile != "production" {
		t.Errorf("CurrentProfile: want %q, got %q", "production", cfg.CurrentProfile)
	}
}

func TestApplyConfigValue_APIURL(t *testing.T) {
	cfg := newTestConfig("default", "text")
	applyConfigValue(cfg, configKeyAPIURL, "https://staging.example.com")

	profile := cfg.Profiles["default"]
	if profile.APIURL != "https://staging.example.com" {
		t.Errorf("APIURL: want %q, got %q", "https://staging.example.com", profile.APIURL)
	}
}

func TestApplyConfigValue_Output(t *testing.T) {
	cfg := newTestConfig("default", "text")
	applyConfigValue(cfg, configKeyOutput, "json")

	if cfg.Output != "json" {
		t.Errorf("Output: want %q, got %q", "json", cfg.Output)
	}
}

func TestApplyConfigValue_Mode(t *testing.T) {
	cfg := newTestConfig("default", "text")
	applyConfigValue(cfg, configKeyMode, "live")

	profile := cfg.Profiles["default"]
	if profile.Mode != "live" {
		t.Errorf("Mode: want %q, got %q", "live", profile.Mode)
	}
}

func TestApplyConfigValue_CreatesProfileWhenMissing(t *testing.T) {
	cfg := newTestConfig("", "text")
	applyConfigValue(cfg, configKeyAPIURL, "https://x.example.com")

	if _, exists := cfg.Profiles["default"]; !exists {
		t.Error("expected fallback profile 'default' to be created")
	}
}

func TestResolveConfigValue_FallsBackToDefaults(t *testing.T) {
	cfg := newTestConfig("default", "")

	if got := resolveConfigValue(cfg, configKeyAPIURL); got != config.DefaultAPIURL {
		t.Errorf("APIURL fallback: want %q, got %q", config.DefaultAPIURL, got)
	}
	if got := resolveConfigValue(cfg, configKeyOutput); got != config.DefaultOutput {
		t.Errorf("Output fallback: want %q, got %q", config.DefaultOutput, got)
	}
	if got := resolveConfigValue(cfg, configKeyMode); got != config.DefaultMode {
		t.Errorf("Mode fallback: want %q, got %q", config.DefaultMode, got)
	}
}

func TestResolveConfigValue_ReturnsExplicitValuesWhenSet(t *testing.T) {
	cfg := newTestConfig("prod", "json")
	cfg.Profiles["prod"] = config.Profile{
		APIURL: "https://prod.example.com",
		Mode:   "live",
	}

	if got := resolveConfigValue(cfg, configKeyProfile); got != "prod" {
		t.Errorf("Profile: want %q, got %q", "prod", got)
	}
	if got := resolveConfigValue(cfg, configKeyAPIURL); got != "https://prod.example.com" {
		t.Errorf("APIURL: want %q, got %q", "https://prod.example.com", got)
	}
	if got := resolveConfigValue(cfg, configKeyOutput); got != "json" {
		t.Errorf("Output: want %q, got %q", "json", got)
	}
	if got := resolveConfigValue(cfg, configKeyMode); got != "live" {
		t.Errorf("Mode: want %q, got %q", "live", got)
	}
}

func TestResolveConfigValue_UnknownKeyReturnsEmpty(t *testing.T) {
	cfg := newTestConfig("default", "text")
	if got := resolveConfigValue(cfg, "nope"); got != "" {
		t.Errorf("unknown key: want empty, got %q", got)
	}
}

func TestValidConfigKeys_ContainsAllPublicKeys(t *testing.T) {
	for _, key := range []string{configKeyProfile, configKeyAPIURL, configKeyOutput, configKeyMode} {
		if !validConfigKeys[key] {
			t.Errorf("%q should be in validConfigKeys", key)
		}
	}
}

func TestRunConfigList_RendersWithFakeHome(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{CurrentProfile: "default", Output: "text", Profiles: map[string]config.Profile{"default": {APIURL: "https://x"}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runConfigList(nil, nil); err != nil {
		t.Errorf("runConfigList: %v", err)
	}
}

func TestRunConfigSet_PersistsValue(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{CurrentProfile: "default", Profiles: map[string]config.Profile{"default": {}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runConfigSet(nil, []string{configKeyAPIURL, "https://staging.example.com"}); err != nil {
		t.Fatalf("runConfigSet: %v", err)
	}
	reloaded, _ := config.Load()
	if reloaded.Profiles["default"].APIURL != "https://staging.example.com" {
		t.Errorf("APIURL not persisted: %q", reloaded.Profiles["default"].APIURL)
	}
}

func TestRunConfigSet_RejectsUnknownKey(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{CurrentProfile: "default", Profiles: map[string]config.Profile{"default": {}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runConfigSet(nil, []string{"nope", "x"}); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestRunConfigGet_ReadsValue(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Output:         "json",
		Profiles:       map[string]config.Profile{"default": {APIURL: "https://x.example.com", Mode: "live"}},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runConfigGet(nil, []string{configKeyOutput}); err != nil {
		t.Errorf("runConfigGet: %v", err)
	}
}

func TestRunConfigGet_RejectsUnknownKey(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{CurrentProfile: "default", Profiles: map[string]config.Profile{"default": {}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runConfigGet(nil, []string{"nope"}); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestPrintConfigValues_DoesNotPanic(t *testing.T) {
	cfg := &config.Config{
		CurrentProfile: "default",
		Output:         "json",
		Profiles: map[string]config.Profile{
			"default": {APIURL: "https://x", Mode: "live"},
		},
	}
	printConfigValues(cfg)
}

func TestPrintConfigValues_FallbacksRender(t *testing.T) {
	cfg := &config.Config{Profiles: map[string]config.Profile{}}
	printConfigValues(cfg)
}
