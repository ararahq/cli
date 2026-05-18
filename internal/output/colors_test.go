package output

import (
	"strings"
	"testing"
)

func TestParseThemeName(t *testing.T) {
	cases := []struct {
		input string
		want  ThemeName
	}{
		{"", ThemeAuto},
		{" auto ", ThemeAuto},
		{"AUTO", ThemeAuto},
		{"mono", ThemeMono},
		{"none", ThemeMono},
		{"OFF", ThemeMono},
		{"no", ThemeMono},
		{"banana", ThemeAuto},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := ParseThemeName(tc.input); got != tc.want {
				t.Errorf("ParseThemeName(%q): want %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestApplyTheme_MonoStripsColors(t *testing.T) {
	original := ActiveTheme()
	t.Cleanup(func() { ApplyTheme(original) })

	ApplyTheme(ThemeMono)
	if got := ActiveTheme(); got != ThemeMono {
		t.Errorf("ActiveTheme: want mono, got %q", got)
	}

	rendered := SuccessStyle.Render("ok")
	if rendered != "ok" {
		t.Errorf("mono SuccessStyle should not apply ANSI: got %q", rendered)
	}
	if strings.Contains(rendered, "\x1b[") {
		t.Errorf("mono output must not contain ANSI escapes: %q", rendered)
	}
}

func TestApplyTheme_AutoRestoresStyles(t *testing.T) {
	original := ActiveTheme()
	t.Cleanup(func() { ApplyTheme(original) })

	ApplyTheme(ThemeMono)
	ApplyTheme(ThemeAuto)
	if got := ActiveTheme(); got != ThemeAuto {
		t.Errorf("expected auto theme, got %q", got)
	}
}

func TestResolveInitialTheme(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("ARARA_THEME", "")
	if got := resolveInitialTheme(); got != ThemeMono {
		t.Errorf("NO_COLOR set should force mono, got %q", got)
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("ARARA_THEME", "mono")
	if got := resolveInitialTheme(); got != ThemeMono {
		t.Errorf("ARARA_THEME=mono should yield mono, got %q", got)
	}

	t.Setenv("ARARA_THEME", "")
	if got := resolveInitialTheme(); got != ThemeAuto {
		t.Errorf("no env vars should yield auto, got %q", got)
	}
}

func TestPrintHeader_DoesNotPanicWithMono(t *testing.T) {
	original := ActiveTheme()
	t.Cleanup(func() { ApplyTheme(original) })

	ApplyTheme(ThemeMono)
	PrintHeader()
}
