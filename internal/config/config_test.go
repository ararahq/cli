package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTempHome(t *testing.T) string {
	t.Helper()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	return tempHome
}

func TestConfigPath_HonorsHome(t *testing.T) {
	tempHome := setupTempHome(t)
	got := ConfigPath()
	want := filepath.Join(tempHome, ConfigDir, ConfigFileName)
	if got != want {
		t.Errorf("ConfigPath: want %q, got %q", want, got)
	}
}

func TestLoad_CreatesDefaultWhenMissing(t *testing.T) {
	setupTempHome(t)

	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if configuration.CurrentProfile != defaultProfileName {
		t.Errorf("default current profile: want %q, got %q", defaultProfileName, configuration.CurrentProfile)
	}
	if configuration.Output != DefaultOutput {
		t.Errorf("default output: want %q, got %q", DefaultOutput, configuration.Output)
	}
	defaultProfile, exists := configuration.Profiles[defaultProfileName]
	if !exists {
		t.Fatal("default profile missing")
	}
	if defaultProfile.APIURL != DefaultAPIURL || defaultProfile.Mode != DefaultMode {
		t.Errorf("default profile values incorrect: %+v", defaultProfile)
	}

	if _, statErr := os.Stat(ConfigPath()); statErr != nil {
		t.Errorf("config file should be created on first Load: %v", statErr)
	}
}

func TestLoad_ReadsExistingFile(t *testing.T) {
	setupTempHome(t)

	custom := &Config{
		CurrentProfile: "prod",
		Output:         "json",
		Profiles: map[string]Profile{
			"prod": {APIKey: "ara_live_x", APIURL: "https://prod.example.com", Mode: "live"},
		},
	}
	if err := Save(custom); err != nil {
		t.Fatalf("Save: %v", err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if configuration.CurrentProfile != "prod" {
		t.Errorf("current profile not preserved: %q", configuration.CurrentProfile)
	}
	if configuration.Output != "json" {
		t.Errorf("output not preserved: %q", configuration.Output)
	}
	if configuration.Profiles["prod"].APIKey != "ara_live_x" {
		t.Errorf("profile fields lost in roundtrip: %+v", configuration.Profiles["prod"])
	}
}

func TestLoad_RejectsInvalidYAML(t *testing.T) {
	setupTempHome(t)

	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(), []byte("::: not yaml :::"), 0o600); err != nil {
		t.Fatalf("write bad yaml: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Error("expected error parsing invalid YAML, got nil")
	}
}

func TestLoad_NormalizesEmptyFields(t *testing.T) {
	setupTempHome(t)

	if err := EnsureConfigDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(), []byte("current_profile: foo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if configuration.Profiles == nil {
		t.Error("Profiles map should be initialized when missing")
	}
	if configuration.Output != DefaultOutput {
		t.Errorf("Output should default to %q, got %q", DefaultOutput, configuration.Output)
	}
}

func TestSave_WritesWithSecurePermissions(t *testing.T) {
	setupTempHome(t)

	configuration := &Config{
		CurrentProfile: "default",
		Profiles:       map[string]Profile{"default": {APIURL: DefaultAPIURL, Mode: DefaultMode}},
	}
	if err := Save(configuration); err != nil {
		t.Fatalf("Save: %v", err)
	}

	stat, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if mode := stat.Mode().Perm(); mode != 0o600 {
		t.Errorf("config file permission: want 0600, got %o", mode)
	}

	dirStat, err := os.Stat(filepath.Dir(ConfigPath()))
	if err != nil {
		t.Fatal(err)
	}
	if mode := dirStat.Mode().Perm(); mode != 0o700 {
		t.Errorf("config dir permission: want 0700, got %o", mode)
	}
}

func TestGetActiveProfile_HappyPath(t *testing.T) {
	configuration := &Config{
		CurrentProfile: "p",
		Profiles: map[string]Profile{
			"p": {APIURL: "https://x", Mode: "live", APIKey: "k"},
		},
	}
	got, err := GetActiveProfile(configuration)
	if err != nil {
		t.Fatalf("GetActiveProfile: %v", err)
	}
	if got.APIURL != "https://x" || got.Mode != "live" {
		t.Errorf("unexpected active profile: %+v", got)
	}
}

func TestGetActiveProfile_NoCurrentProfile(t *testing.T) {
	configuration := &Config{Profiles: map[string]Profile{}}
	if _, err := GetActiveProfile(configuration); err == nil {
		t.Error("expected error when current profile empty")
	}
}

func TestGetActiveProfile_ProfileMissing(t *testing.T) {
	configuration := &Config{
		CurrentProfile: "ghost",
		Profiles:       map[string]Profile{"other": {}},
	}
	_, err := GetActiveProfile(configuration)
	if err == nil {
		t.Fatal("expected error when current profile not in map")
	}
	if got := err.Error(); !strings.Contains(got, "ghost") || !strings.Contains(got, "other") {
		t.Errorf("error should name missing and available profiles, got %q", got)
	}
}

func TestGetActiveProfile_FillsDefaults(t *testing.T) {
	configuration := &Config{
		CurrentProfile: "p",
		Profiles:       map[string]Profile{"p": {}},
	}
	got, err := GetActiveProfile(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIURL != DefaultAPIURL {
		t.Errorf("APIURL default: want %q, got %q", DefaultAPIURL, got.APIURL)
	}
	if got.Mode != DefaultMode {
		t.Errorf("Mode default: want %q, got %q", DefaultMode, got.Mode)
	}
}

func TestAvailableProfileNames(t *testing.T) {
	if got := availableProfileNames(&Config{}); got != "(none)" {
		t.Errorf("empty profiles should yield (none), got %q", got)
	}
	got := availableProfileNames(&Config{Profiles: map[string]Profile{"a": {}, "b": {}}})
	if got != "a, b" && got != "b, a" {
		t.Errorf("expected joined names, got %q", got)
	}
}
