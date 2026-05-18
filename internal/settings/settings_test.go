package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setupSettingsHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func writeUserSettings(t *testing.T, content string) {
	t.Helper()
	path := UserSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeProjectSettings(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ProjectSettingsDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ProjectSettingsDirName, ProjectSettingsFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_DefaultsOnly(t *testing.T) {
	setupSettingsHome(t)
	resolved, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if resolved.Settings.Theme != "auto" {
		t.Errorf("default theme: want auto, got %q", resolved.Settings.Theme)
	}
	if resolved.Settings.HookTimeoutSeconds != defaultHookTimeoutSeconds {
		t.Errorf("default hookTimeoutSeconds: want %d, got %d", defaultHookTimeoutSeconds, resolved.Settings.HookTimeoutSeconds)
	}
	if len(resolved.Sources) != 1 || resolved.Sources[0].Name != SourceDefault {
		t.Errorf("expected only default source, got %+v", resolved.Sources)
	}
}

func TestLoad_UserOverridesDefaults(t *testing.T) {
	setupSettingsHome(t)
	writeUserSettings(t, `{"theme":"mono","hookTimeoutSeconds":15}`)

	resolved, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if resolved.Settings.Theme != "mono" {
		t.Errorf("user theme should win: got %q", resolved.Settings.Theme)
	}
	if resolved.Settings.HookTimeoutSeconds != 15 {
		t.Errorf("user hookTimeoutSeconds should win: got %d", resolved.Settings.HookTimeoutSeconds)
	}

	if !sourceFor(resolved, "theme", SourceUser) {
		t.Error("theme provenance should be user")
	}
}

func TestLoad_ProjectOverridesUser(t *testing.T) {
	setupSettingsHome(t)
	writeUserSettings(t, `{"theme":"mono","hooks":{"preSend":["/u/hook"]}}`)

	projectDir := t.TempDir()
	writeProjectSettings(t, projectDir, `{"theme":"auto","hooks":{"preSend":["/p/hook"]}}`)

	resolved, err := Load(projectDir)
	if err != nil {
		t.Fatal(err)
	}

	if resolved.Settings.Theme != "auto" {
		t.Errorf("project theme should win: got %q", resolved.Settings.Theme)
	}
	wantedHooks := []string{"/u/hook", "/p/hook"}
	if !reflect.DeepEqual(resolved.Settings.Hooks.PreSend, wantedHooks) {
		t.Errorf("hooks.preSend should concat user+project, got %v", resolved.Settings.Hooks.PreSend)
	}
}

func TestLoad_ProjectFromAncestorDirectory(t *testing.T) {
	setupSettingsHome(t)
	root := t.TempDir()
	writeProjectSettings(t, root, `{"theme":"mono"}`)
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := Load(deep)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Settings.Theme != "mono" {
		t.Errorf("expected to walk up and find project settings, got theme=%q", resolved.Settings.Theme)
	}
}

func TestLoad_InvalidUserJSONIsIgnored(t *testing.T) {
	setupSettingsHome(t)
	writeUserSettings(t, `not json`)

	resolved, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load should not error on bad user file, got: %v", err)
	}
	if resolved.Settings.Theme != "auto" {
		t.Errorf("invalid user file should fall back to defaults, got theme=%q", resolved.Settings.Theme)
	}
}

func TestWriteValue_UserScope(t *testing.T) {
	setupSettingsHome(t)

	if err := WriteValue(ScopeUser, "", "theme", "mono"); err != nil {
		t.Fatal(err)
	}

	bytes, err := os.ReadFile(UserSettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var parsed Settings
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Theme != "mono" {
		t.Errorf("expected theme=mono in user file, got %q", parsed.Theme)
	}
}

func TestWriteValue_ProjectScope(t *testing.T) {
	setupSettingsHome(t)
	projectDir := t.TempDir()

	if err := WriteValue(ScopeProject, projectDir, "hooks.preSend", []string{"/x/hook"}); err != nil {
		t.Fatal(err)
	}

	expectedPath := filepath.Join(projectDir, ProjectSettingsDirName, ProjectSettingsFileName)
	bytes, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed Settings
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Hooks.PreSend, []string{"/x/hook"}) {
		t.Errorf("expected preSend=[/x/hook], got %v", parsed.Hooks.PreSend)
	}
}

func TestWriteValue_RejectsUnknownPath(t *testing.T) {
	setupSettingsHome(t)
	if err := WriteValue(ScopeUser, "", "unknown.path", "x"); err == nil {
		t.Error("expected error on unknown path")
	}
}

func TestWriteValue_TypeMismatch(t *testing.T) {
	setupSettingsHome(t)
	if err := WriteValue(ScopeUser, "", "theme", 42); err == nil {
		t.Error("expected error when assigning int to string field")
	}
	if err := WriteValue(ScopeUser, "", "hookTimeoutSeconds", "not-a-number"); err == nil {
		t.Error("expected error when assigning bad value to int field")
	}
	if err := WriteValue(ScopeUser, "", "experimental.oauth", "yes"); err == nil {
		t.Error("expected error when assigning string to bool field")
	}
}

func TestGetValue_UnknownPath(t *testing.T) {
	setupSettingsHome(t)
	if _, _, err := GetValue("", "no.such.path"); err == nil {
		t.Error("expected error on unknown path")
	}
}

func TestGetValue_DefaultsSource(t *testing.T) {
	setupSettingsHome(t)
	value, source, err := GetValue("", "theme")
	if err != nil {
		t.Fatal(err)
	}
	if value != "auto" || source != SourceDefault {
		t.Errorf("default theme: want (auto, default), got (%v, %q)", value, source)
	}
}

func TestGetValue_UserSource(t *testing.T) {
	setupSettingsHome(t)
	writeUserSettings(t, `{"theme":"mono"}`)

	value, source, err := GetValue("", "theme")
	if err != nil {
		t.Fatal(err)
	}
	if value != "mono" || source != SourceUser {
		t.Errorf("expected (mono, user), got (%v, %q)", value, source)
	}
}

func TestAllKnownPaths_ApplyAndLookupRoundtrip(t *testing.T) {
	setupSettingsHome(t)

	cases := []struct {
		path     string
		writeVal any
		wantVal  any
	}{
		{"theme", "mono", "mono"},
		{"output", "json", "json"},
		{"hookTimeoutSeconds", 99, 99},
		{"hooks.preSend", []string{"a"}, []string{"a"}},
		{"hooks.postDeliver", []string{"b"}, []string{"b"}},
		{"hooks.preCampaign", []string{"c"}, []string{"c"}},
		{"hooks.postCampaign", []string{"d"}, []string{"d"}},
		{"hooks.onError", []string{"e"}, []string{"e"}},
		{"plugins.searchPaths", []string{"/p"}, []string{"/p"}},
		{"plugins.disabled", []string{"foo"}, []string{"foo"}},
		{"experimental.oauth", true, true},
		{"mcp.allowWriteTools", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if err := WriteValue(ScopeUser, "", tc.path, tc.writeVal); err != nil {
				t.Fatalf("WriteValue %s: %v", tc.path, err)
			}
			got, source, err := GetValue("", tc.path)
			if err != nil {
				t.Fatalf("GetValue %s: %v", tc.path, err)
			}
			if !reflect.DeepEqual(got, tc.wantVal) {
				t.Errorf("%s roundtrip: want %v, got %v", tc.path, tc.wantVal, got)
			}
			if source != SourceUser {
				t.Errorf("%s source: want user, got %q", tc.path, source)
			}
		})
	}
}

func TestProjectSettingsPath(t *testing.T) {
	setupSettingsHome(t)
	root := t.TempDir()
	writeProjectSettings(t, root, `{}`)

	got := ProjectSettingsPath(root)
	want := filepath.Join(root, ProjectSettingsDirName, ProjectSettingsFileName)
	if got != want {
		t.Errorf("ProjectSettingsPath: want %q, got %q", want, got)
	}

	if got := ProjectSettingsPath(t.TempDir()); got != "" {
		t.Errorf("expected empty path when no project settings exist, got %q", got)
	}
}

func TestResolveWritePath_UnknownScope(t *testing.T) {
	if _, err := resolveWritePath(WriteScope("nope"), ""); err == nil {
		t.Error("expected error on unknown scope")
	}
}

func TestKnownPaths(t *testing.T) {
	paths := KnownPaths()
	if len(paths) == 0 {
		t.Fatal("KnownPaths should not be empty")
	}
	required := []string{"theme", "hooks.preSend", "experimental.oauth"}
	for _, want := range required {
		found := false
		for _, got := range paths {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("KnownPaths missing %q", want)
		}
	}
}

func TestToStringSlice(t *testing.T) {
	if got := toStringSlice([]string{"a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("[]string passthrough failed: %v", got)
	}
	if got := toStringSlice("a, b , ,c"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("string parse failed: %v", got)
	}
	if got := toStringSlice(""); got != nil {
		t.Errorf("empty string should yield nil, got %v", got)
	}
	if got := toStringSlice([]any{"a", 1, "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("[]any with non-strings should filter, got %v", got)
	}
	if got := toStringSlice(42); got != nil {
		t.Errorf("unsupported type should yield nil, got %v", got)
	}
}

func TestToInt(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  int
		ok    bool
	}{
		{"int", 42, 42, true},
		{"int64", int64(7), 7, true},
		{"float64", 3.5, 3, true},
		{"numeric string", "  9  ", 9, true},
		{"non-numeric string", "banana", 0, false},
		{"slice", []int{1}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toInt(tc.input)
			if got != tc.want || ok != tc.ok {
				t.Errorf("toInt(%v): want (%d,%v), got (%d,%v)", tc.input, tc.want, tc.ok, got, ok)
			}
		})
	}
}

func TestAppendUnique(t *testing.T) {
	got := appendUnique([]string{"a", "b"}, []string{"b", "c"})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("expected dedupe, got %v", got)
	}
}

func sourceFor(resolved *Resolved, path, want string) bool {
	for _, field := range resolved.Fields {
		if field.Path == path && field.Source == want {
			return true
		}
	}
	return false
}
