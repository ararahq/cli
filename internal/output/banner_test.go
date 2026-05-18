package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderMascot_HasFourLines(t *testing.T) {
	// New mascot mirrors the official logo: crest + head/beak + body + tail.
	rows := strings.Split(RenderMascot(), "\n")
	if len(rows) != 4 {
		t.Errorf("mascot should be exactly 4 lines, got %d", len(rows))
	}
}

func TestRenderMascot_MonoIsPlainText(t *testing.T) {
	original := ActiveTheme()
	t.Cleanup(func() { ApplyTheme(original) })
	ApplyTheme(ThemeMono)

	got := RenderMascot()
	if strings.Contains(got, "\x1b[") {
		t.Errorf("mono mascot must not contain ANSI escapes, got %q", got)
	}
}

func TestRenderWordmark_HasTwoLines(t *testing.T) {
	rows := strings.Split(RenderWordmark(), "\n")
	if len(rows) != 2 {
		t.Errorf("wordmark should be exactly 2 lines, got %d", len(rows))
	}
}

func TestRenderBanner_IncludesVersionAndSubtitle(t *testing.T) {
	got := RenderBanner("0.2.0")
	if !strings.Contains(got, "0.2.0") {
		t.Errorf("banner should include version, got:\n%s", got)
	}
	if !strings.Contains(got, "WhatsApp API Platform") {
		t.Errorf("banner should include subtitle, got:\n%s", got)
	}
	if !strings.Contains(got, "ararahq.com") {
		t.Errorf("banner should include domain, got:\n%s", got)
	}
}

func TestRenderBanner_MonoSafe(t *testing.T) {
	original := ActiveTheme()
	t.Cleanup(func() { ApplyTheme(original) })
	ApplyTheme(ThemeMono)

	got := RenderBanner("0.2.0")
	if strings.Contains(got, "\x1b[") {
		t.Errorf("mono banner must not contain ANSI escapes")
	}
	if !strings.Contains(got, "0.2.0") {
		t.Errorf("mono banner should still include version: %q", got)
	}
}

func TestWriteBanner_PrintsToWriter(t *testing.T) {
	buffer := &bytes.Buffer{}
	WriteBanner(buffer, "0.0.0")
	if buffer.Len() == 0 {
		t.Error("WriteBanner should produce output")
	}
}

func TestRowOrPad_ReturnsRowWhenInRange(t *testing.T) {
	rows := []string{"hello", "world"}
	if got := rowOrPad(rows, 0, 5); got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestRowOrPad_ReturnsPadWhenOutOfRange(t *testing.T) {
	got := rowOrPad([]string{"only-one"}, 5, 4)
	if got != "    " {
		t.Errorf("expected 4-space pad, got %q", got)
	}
}
