package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/config"
)

func TestBuildProfileSummaries_SortedAndMarksActive(t *testing.T) {
	cfg := &config.Config{
		CurrentProfile: "production",
		Profiles: map[string]config.Profile{
			"sandbox":    {APIURL: "https://api-sandbox.example.com", Mode: "test", APIKey: "ara_test_xxx"},
			"production": {APIURL: "https://api.example.com", Mode: "live", APIKey: "ara_live_yyy"},
			"client-x":   {APIURL: "", Mode: ""},
		},
	}

	summaries := buildProfileSummaries(cfg)

	if len(summaries) != 3 {
		t.Fatalf("want 3 summaries, got %d", len(summaries))
	}
	if summaries[0].Name != "client-x" || summaries[1].Name != "production" || summaries[2].Name != "sandbox" {
		t.Errorf("not sorted: %+v", summaries)
	}

	for _, summary := range summaries {
		if summary.Name == "production" && !summary.Active {
			t.Error("production should be marked active")
		}
		if summary.Name != "production" && summary.Active {
			t.Errorf("%s should not be active", summary.Name)
		}
	}
}

func TestBuildProfileSummaries_AppliesDefaultsForBlankFields(t *testing.T) {
	cfg := &config.Config{
		CurrentProfile: "x",
		Profiles: map[string]config.Profile{
			"x": {APIURL: "", Mode: ""},
		},
	}

	summaries := buildProfileSummaries(cfg)
	if summaries[0].APIURL != config.DefaultAPIURL {
		t.Errorf("APIURL fallback failed: %q", summaries[0].APIURL)
	}
	if summaries[0].Mode != config.DefaultMode {
		t.Errorf("Mode fallback failed: %q", summaries[0].Mode)
	}
}

func TestBuildProfileSummaries_HasKeyDetectsInlineCredential(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"with-key":    {APIKey: "ara_test_abcdefgh1234"},
			"without-key": {},
		},
	}

	summaries := buildProfileSummaries(cfg)

	byName := map[string]bool{}
	for _, summary := range summaries {
		byName[summary.Name] = summary.HasKey
	}
	if !byName["with-key"] {
		t.Error("inline APIKey should report HasKey=true")
	}
	if byName["without-key"] {
		t.Error("missing APIKey should report HasKey=false")
	}
}

func TestPrintProfileSummaries_EmptyShowsHint(t *testing.T) {
	// Captures the printed hint via the printInfo path which writes stdout
	// — at minimum we ensure no panic.
	printProfileSummaries(&bytes.Buffer{}, nil)
}

func TestPrintProfileSummaries_RendersAllRows(t *testing.T) {
	summaries := []profileSummary{
		{Name: "alpha", Active: true, APIURL: "https://a.example.com", Mode: "live", HasKey: true},
		{Name: "beta", Active: false, APIURL: "https://b.example.com", Mode: "test", HasKey: false},
	}
	buffer := &bytes.Buffer{}

	printProfileSummaries(buffer, summaries)

	rendered := buffer.String()
	for _, expected := range []string{"alpha", "beta", "https://a.example.com", "https://b.example.com", "live", "test"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestPickAnyProfile_EmptyMapReturnsEmpty(t *testing.T) {
	cfg := &config.Config{Profiles: map[string]config.Profile{}}
	if got := pickAnyProfile(cfg); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestPickAnyProfile_ReturnsOneOfThem(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"a": {},
			"b": {},
		},
	}
	got := pickAnyProfile(cfg)
	if got != "a" && got != "b" {
		t.Errorf("expected a or b, got %q", got)
	}
}

func TestProfileNamesForError_SortedAndJoined(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{"c": {}, "a": {}, "b": {}},
	}
	if got := profileNamesForError(cfg); got != "a, b, c" {
		t.Errorf("want sorted joined names, got %q", got)
	}
}

func TestProfileNamesForError_EmptyShowsNone(t *testing.T) {
	cfg := &config.Config{Profiles: map[string]config.Profile{}}
	if got := profileNamesForError(cfg); got != "(none)" {
		t.Errorf("want '(none)', got %q", got)
	}
}

func TestNonEmpty(t *testing.T) {
	cases := []struct {
		input    string
		fallback string
		want     string
	}{
		{"", "fallback", "fallback"},
		{"   ", "fallback", "fallback"},
		{"value", "fallback", "value"},
		{" value ", "fallback", " value "},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := nonEmpty(tc.input, tc.fallback); got != tc.want {
				t.Errorf("nonEmpty(%q, %q): want %q, got %q", tc.input, tc.fallback, tc.want, got)
			}
		})
	}
}

// withFakeHome points HOME at a tempdir for the duration of a test so config
// load/save round-trip lands under t.TempDir() and never touches the real
// ~/.arara/. config.Load uses os.UserHomeDir which respects $HOME on Unix.
func withFakeHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestRunProfileList_RendersConfiguredProfiles(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "production",
		Output:         "text",
		Profiles: map[string]config.Profile{
			"production": {APIURL: "https://api.example.com", Mode: "live"},
			"sandbox":    {APIURL: "https://api-sandbox.example.com", Mode: "test"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := runProfileList(nil, nil); err != nil {
		t.Errorf("runProfileList: %v", err)
	}
}

func TestRunProfileSwitch_ChangesActive(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIURL: "https://api.example.com"},
			"prod":    {APIURL: "https://prod.example.com"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := runProfileSwitch(nil, []string{"prod"}); err != nil {
		t.Fatalf("runProfileSwitch: %v", err)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CurrentProfile != "prod" {
		t.Errorf("CurrentProfile after switch: want %q, got %q", "prod", reloaded.CurrentProfile)
	}
}

func TestRunProfileSwitch_FailsForUnknownProfile(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles:       map[string]config.Profile{"default": {}},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	err := runProfileSwitch(nil, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention name, got: %v", err)
	}
}

func TestRunProfileAdd_CreatesAndSetsDefaults(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles:       map[string]config.Profile{"default": {}},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := runProfileAdd(nil, []string{"client-x"}); err != nil {
		t.Fatalf("runProfileAdd: %v", err)
	}

	reloaded, _ := config.Load()
	clientX, exists := reloaded.Profiles["client-x"]
	if !exists {
		t.Fatal("client-x not present after add")
	}
	if clientX.APIURL != config.DefaultAPIURL {
		t.Errorf("APIURL default not applied: %q", clientX.APIURL)
	}
	if clientX.Mode != config.DefaultMode {
		t.Errorf("Mode default not applied: %q", clientX.Mode)
	}
}

func TestRunProfileAdd_RejectsDuplicate(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {},
			"prod":    {},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	err := runProfileAdd(nil, []string{"prod"})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestRunProfileRemove_DeletesAndPicksAnother(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "prod",
		Profiles: map[string]config.Profile{
			"default": {},
			"prod":    {},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := runProfileRemove(nil, []string{"prod"}); err != nil {
		t.Fatalf("runProfileRemove: %v", err)
	}

	reloaded, _ := config.Load()
	if _, exists := reloaded.Profiles["prod"]; exists {
		t.Error("prod should be removed")
	}
	if reloaded.CurrentProfile != "default" {
		t.Errorf("active profile should fall back to default, got %q", reloaded.CurrentProfile)
	}
}

func TestRunProfileRemove_FailsForUnknown(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles:       map[string]config.Profile{"default": {}},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	err := runProfileRemove(nil, []string{"missing"})
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}
