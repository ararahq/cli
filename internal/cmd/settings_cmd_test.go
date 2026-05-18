package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSettingsValue_StringPaths(t *testing.T) {
	for _, path := range []string{"theme", "output"} {
		got, err := parseSettingsValue(path, "mono")
		if err != nil {
			t.Fatalf("parseSettingsValue(%q): %v", path, err)
		}
		if got != "mono" {
			t.Errorf("%q: want %q, got %v", path, "mono", got)
		}
	}
}

func TestParseSettingsValue_HookTimeoutSeconds(t *testing.T) {
	got, err := parseSettingsValue("hookTimeoutSeconds", "45")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != 45 {
		t.Errorf("want 45, got %v", got)
	}

	if _, err := parseSettingsValue("hookTimeoutSeconds", "not-a-number"); err == nil {
		t.Error("expected error parsing non-int")
	}
}

func TestParseSettingsValue_BooleanPaths(t *testing.T) {
	cases := map[string]bool{"true": true, "false": false, "1": true, "0": false}
	for raw, want := range cases {
		got, err := parseSettingsValue("experimental.oauth", raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if got != want {
			t.Errorf("%q: want %v, got %v", raw, want, got)
		}
	}

	if _, err := parseSettingsValue("mcp.allowWriteTools", "yes"); err == nil {
		t.Error("expected error on non-bool")
	}
}

func TestParseSettingsValue_ListPathsCommaSeparated(t *testing.T) {
	got, err := parseSettingsValue("hooks.preSend", "/a/b.sh, /c/d.sh ,/e/f.sh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"/a/b.sh", "/c/d.sh", "/e/f.sh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestParseSettingsValue_ListPathsJSONArray(t *testing.T) {
	got, err := parseSettingsValue("plugins.searchPaths", `["/usr/local/bin","/opt/bin"]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"/usr/local/bin", "/opt/bin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestParseSettingsValue_ListPathsInvalidJSON(t *testing.T) {
	_, err := parseSettingsValue("plugins.disabled", `[broken`)
	if err == nil {
		t.Error("expected error on broken JSON array")
	}
}

func TestParseSettingsValue_UnknownPath(t *testing.T) {
	_, err := parseSettingsValue("not.a.real.setting", "x")
	if err == nil {
		t.Fatal("expected error for unknown path")
	}
	if !strings.Contains(err.Error(), "unknown setting path") {
		t.Errorf("error should mention unknown path, got: %v", err)
	}
}

func TestSplitAndTrim(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
		{"single-no-spaces", []string{"single-no-spaces"}},
		{"with-trailing-comma,", []string{"with-trailing-comma"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := splitAndTrim(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("splitAndTrim(%q): want len %d (%v), got len %d (%v)", tc.input, len(tc.want), tc.want, len(got), got)
			}
			for index := range got {
				if got[index] != tc.want[index] {
					t.Errorf("splitAndTrim(%q)[%d]: want %q, got %q", tc.input, index, tc.want[index], got[index])
				}
			}
		})
	}

	// All-blank input still returns an empty result (slice or nil, both fine).
	if got := splitAndTrim("   ,  ,   "); len(got) != 0 {
		t.Errorf("all-blank input should yield empty, got %v", got)
	}
}

func TestFormatSettingsValue(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "mono", "mono"},
		{"empty list", []string{}, "[]"},
		{"non-empty list", []string{"a", "b"}, "[a, b]"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"int", 42, "42"},
		{"map", map[string]int{"a": 1}, `{"a":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatSettingsValue(tc.input); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}
