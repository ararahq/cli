package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

func TestBuildOrgRows_SortsAndMarksActive(t *testing.T) {
	orgs := []api.Organization{
		{ID: "1", Slug: "zeta", Name: "Zeta Corp", Role: "OWNER", Mode: "live"},
		{ID: "2", Slug: "alpha", Name: "Alpha Inc", Role: "MEMBER", Mode: "test"},
		{ID: "3", Slug: "beta", Name: "Beta LLC", Role: "OWNER", Mode: "live"},
	}

	rows := buildOrgRows(orgs, "beta")

	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	expectedOrder := []string{"alpha", "beta", "zeta"}
	for i, want := range expectedOrder {
		if rows[i].Slug != want {
			t.Errorf("rows[%d].Slug: want %q, got %q", i, want, rows[i].Slug)
		}
	}
	for _, row := range rows {
		if row.Slug == "beta" && !row.Active {
			t.Error("beta should be marked active")
		}
		if row.Slug != "beta" && row.Active {
			t.Errorf("%s should not be marked active", row.Slug)
		}
	}
}

func TestBuildOrgRows_EmptyInput(t *testing.T) {
	rows := buildOrgRows(nil, "any")
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestPrintOrgRows_RendersAllFields(t *testing.T) {
	rows := []orgRow{
		{Slug: "acme", Name: "Acme Corp", Role: "OWNER", Mode: "live", Active: true},
		{Slug: "client-x", Name: "Cliente X", Role: "MEMBER", Mode: "test", Active: false},
	}
	buffer := &bytes.Buffer{}

	printOrgRows(buffer, rows)

	rendered := buffer.String()
	for _, expected := range []string{"acme", "Acme Corp", "live", "OWNER", "client-x", "Cliente X", "test", "MEMBER"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestFindOrgBySlug(t *testing.T) {
	orgs := []api.Organization{
		{Slug: "alpha", Name: "Alpha"},
		{Slug: "beta", Name: "Beta"},
	}

	if got, ok := findOrgBySlug(orgs, "beta"); !ok || got.Name != "Beta" {
		t.Errorf("find existing failed: got=%+v ok=%v", got, ok)
	}
	if _, ok := findOrgBySlug(orgs, "missing"); ok {
		t.Error("find missing should return ok=false")
	}
}

func TestJoinOrgSlugs(t *testing.T) {
	orgs := []api.Organization{
		{Slug: "zeta"}, {Slug: "alpha"}, {Slug: "beta"},
	}
	if got := joinOrgSlugs(orgs); got != "alpha, beta, zeta" {
		t.Errorf("want sorted joined slugs, got %q", got)
	}
	if got := joinOrgSlugs(nil); got != "(none)" {
		t.Errorf("empty should yield (none), got %q", got)
	}
}

func TestMapOrgModeToProfileMode(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"live", "live"},
		{"LIVE", "live"},
		{"test", "test"},
		{" test ", "test"},
		{"", config.DefaultMode},
		{"weird", config.DefaultMode},
	}
	for _, tc := range cases {
		if got := mapOrgModeToProfileMode(tc.input); got != tc.want {
			t.Errorf("mapOrgModeToProfileMode(%q): want %q, got %q", tc.input, tc.want, got)
		}
	}
}

func TestPromoteOrgToActiveProfile_CreatesAndSwitches(t *testing.T) {
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "ara_live_xxx", APIURL: "https://api.example.com", Mode: "live"},
		},
	}

	promoteOrgToActiveProfile(cfg, api.Organization{Slug: "acme", Name: "Acme", Mode: "live"})

	if cfg.CurrentProfile != "acme" {
		t.Errorf("CurrentProfile: want acme, got %q", cfg.CurrentProfile)
	}
	acme, exists := cfg.Profiles["acme"]
	if !exists {
		t.Fatal("acme profile should be created")
	}
	if acme.APIKey != "ara_live_xxx" {
		t.Errorf("APIKey not inherited: %q", acme.APIKey)
	}
	if acme.APIURL != "https://api.example.com" {
		t.Errorf("APIURL not inherited: %q", acme.APIURL)
	}
}

func TestPromoteOrgToActiveProfile_ReusesExistingProfile(t *testing.T) {
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "ara_live_default", APIURL: "https://default.example.com"},
			"acme":    {APIKey: "ara_live_acme", APIURL: "https://acme.example.com", Mode: "live"},
		},
	}

	promoteOrgToActiveProfile(cfg, api.Organization{Slug: "acme", Name: "Acme", Mode: "live"})

	// Existing profile must be preserved verbatim — don't clobber the
	// per-org key the user previously set up via 'arara login --profile acme'.
	if cfg.Profiles["acme"].APIKey != "ara_live_acme" {
		t.Errorf("existing acme key clobbered: %q", cfg.Profiles["acme"].APIKey)
	}
	if cfg.CurrentProfile != "acme" {
		t.Errorf("CurrentProfile: want acme, got %q", cfg.CurrentProfile)
	}
}

func TestRunOrgListImpl_GoldenPath(t *testing.T) {
	body := `{"data":[
		{"id":"id1","slug":"acme","name":"Acme Corp","role":"OWNER","mode":"live"},
		{"id":"id2","slug":"client-x","name":"Cliente X","role":"MEMBER","mode":"test"}
	]}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runOrgListImpl(client, output.FormatText, buffer, "acme"); err != nil {
		t.Fatalf("runOrgListImpl: %v", err)
	}
	for _, expected := range []string{"acme", "client-x", "Acme Corp", "Cliente X"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunOrgListImpl_EmptyShowsHint(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusOK, `{"data":[]}`)
	buffer := &bytes.Buffer{}

	if err := runOrgListImpl(client, output.FormatText, buffer, ""); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "No organizations") {
		t.Errorf("expected hint, got: %q", buffer.String())
	}
}

func TestRunOrgListImpl_JSONOutput(t *testing.T) {
	body := `{"data":[{"id":"x","slug":"acme","name":"A","role":"OWNER","mode":"live"}]}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runOrgListImpl(client, output.FormatJSON, buffer, ""); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), `"slug"`) || !strings.Contains(buffer.String(), `"acme"`) {
		t.Errorf("JSON missing fields: %q", buffer.String())
	}
}

func TestRunOrgUseImpl_PromotesOrgInConfig(t *testing.T) {
	withFakeHome(t)

	body := `{"data":[{"id":"x","slug":"acme","name":"Acme","role":"OWNER","mode":"live"}]}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)

	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "ara_live_xxx", APIURL: "https://api.example.com"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := runOrgUseImpl(client, "acme"); err != nil {
		t.Fatalf("runOrgUseImpl: %v", err)
	}

	reloaded, _ := config.Load()
	if reloaded.CurrentProfile != "acme" {
		t.Errorf("CurrentProfile not switched: got %q", reloaded.CurrentProfile)
	}
}

func TestRunOrgUseImpl_FailsForUnknownSlug(t *testing.T) {
	withFakeHome(t)

	body := `{"data":[{"id":"x","slug":"acme","name":"Acme","role":"OWNER","mode":"live"}]}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)

	err := runOrgUseImpl(client, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown slug")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention slug, got: %v", err)
	}
}

func TestRunOrgUseImpl_ServerError(t *testing.T) {
	withFakeHome(t)

	_, client := fakeAPIServerJSON(t, http.StatusInternalServerError, `{"error":{"code":"x","message":"y"}}`)

	if err := runOrgUseImpl(client, "acme"); err == nil {
		t.Fatal("expected error from 5xx")
	}
}

func TestRunOrgList_WrapperUsesInjectedClient(t *testing.T) {
	body := `{"data":[{"id":"id1","slug":"acme","name":"Acme","role":"OWNER","mode":"live"}]}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runOrgList(nil, nil); err != nil {
		t.Errorf("runOrgList: %v", err)
	}
}

func TestRunOrgUse_WrapperGoldenPath(t *testing.T) {
	withFakeHome(t)
	cfg := &config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "ara_live_xxx", APIURL: "https://api.example.com"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	body := `{"data":[{"id":"x","slug":"acme","name":"Acme","role":"OWNER","mode":"live"}]}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runOrgUse(nil, []string{"acme"}); err != nil {
		t.Errorf("runOrgUse: %v", err)
	}
}

func TestRunOrgUse_RejectsEmptySlug(t *testing.T) {
	if err := runOrgUse(nil, []string{"   "}); err == nil {
		t.Fatal("expected error on whitespace-only slug")
	}
}
