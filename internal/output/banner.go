package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Mascot is a four-line silhouette of the AraraHQ macaw — head with
// crest, beak pointing right, body, and the long tail trailing diagonally
// down-left, mirroring the official logo (Identidade/logoMarca.png).
// All rows are mascotWidth visible cells so the wordmark composes cleanly
// to the right.
//
// Colour layering:
//   - body uses the brand teal (#1c99a7) from the .pen design tokens
//   - the beak gets the soft accent shade (#aff1f2) so the silhouette
//     reads as a feature
//   - everything degrades cleanly to plain text when ApplyTheme(ThemeMono)
//     is on (NO_COLOR, --no-color, --theme=mono)
const (
	mascotLineCrest = `   ◢▆◣   `
	mascotLineHead  = `  ◢██◤▶  `
	mascotLineBody  = `   ▜█▌   `
	mascotLineTail  = `    ╲╲   `
	mascotWidth     = 9
)

// AraraHQ wordmark in chunky block letters: A R A R A H Q.
const (
	wordmarkLine1 = `█▀█ █▀█ █▀█ █▀█ █▀█ █ █ █▀█`
	wordmarkLine2 = `█▀█ █▀▄ █▀█ █▀▄ █▀█ █▀█ ▀▀█`

	bannerSubtitle = "WhatsApp API Platform · ararahq.com"
)

var (
	bannerBodyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandColorTeal))
	bannerBeakStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandColorAccent))
	bannerSubStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextSecondary))
	bannerVerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandColorTeal))
)

// RenderMascot returns the four-line bird in brand colour, ready to print.
// The beak gets a slightly lighter accent so the silhouette pops without
// breaking the brand teal identity.
func RenderMascot() string {
	if ActiveTheme() == ThemeMono {
		return mascotLineCrest + "\n" + mascotLineHead + "\n" + mascotLineBody + "\n" + mascotLineTail
	}
	// Head row: split off the beak character so it gets the accent style
	// without re-styling the whole line.
	headBody := mascotLineHead[:len(mascotLineHead)-len("▶  ")]
	headTail := "  "
	row1 := bannerBodyStyle.Render(mascotLineCrest)
	row2 := bannerBodyStyle.Render(headBody) + bannerBeakStyle.Render("▶") + headTail
	row3 := bannerBodyStyle.Render(mascotLineBody)
	row4 := bannerBodyStyle.Render(mascotLineTail)
	return row1 + "\n" + row2 + "\n" + row3 + "\n" + row4
}

// RenderWordmark returns the chunky "AraraHQ" block-letter wordmark in
// brand teal. Used on the welcome banner next to the mascot.
func RenderWordmark() string {
	if ActiveTheme() == ThemeMono {
		return wordmarkLine1 + "\n" + wordmarkLine2
	}
	return bannerBodyStyle.Render(wordmarkLine1) + "\n" + bannerBodyStyle.Render(wordmarkLine2)
}

// RenderBanner composes mascot + wordmark side by side, with subtitle and
// version below. This is the canonical splash that `arara` (no args, TTY)
// and `arara --help` show. Stays under 80 columns at default zoom.
//
// Layout:
//
//	 ◢▆◣      █▀█ █▀█ █▀█ █▀█ █▀█ █ █ █▀█
//	◢██◤▶     █▀█ █▀▄ █▀█ █▀▄ █▀█ █▀█ ▀▀█
//	 ▜█▌
//	  ╲╲
//	WhatsApp API Platform · ararahq.com
//	v0.2.0
func RenderBanner(version string) string {
	mascotRows := strings.Split(RenderMascot(), "\n")
	wordmarkRows := strings.Split(RenderWordmark(), "\n")

	bannerRows := make([]string, 0, max(len(mascotRows), len(wordmarkRows)))
	for index := range max(len(mascotRows), len(wordmarkRows)) {
		left := rowOrPad(mascotRows, index, mascotWidth)
		right := rowOrPad(wordmarkRows, index, len(wordmarkLine1))
		bannerRows = append(bannerRows, left+"  "+right)
	}

	subtitle := bannerSubStyle.Render("  " + bannerSubtitle)
	versionLine := bannerSubStyle.Render("  v") + bannerVerStyle.Render(version)
	if ActiveTheme() == ThemeMono {
		subtitle = "  " + bannerSubtitle
		versionLine = "  v" + version
	}

	return strings.Join(bannerRows, "\n") + "\n" + subtitle + "\n" + versionLine + "\n"
}

// WriteBanner writes RenderBanner to the given writer with a trailing
// newline. Use this from places that don't need a return value (init
// hooks, the no-args entrypoint).
func WriteBanner(writer io.Writer, version string) {
	fmt.Fprintln(writer, RenderBanner(version))
}

// rowOrPad returns rows[index] or a fixed-width space pad when the row
// doesn't exist (so banner stays rectangular even if mascot/wordmark
// have different line counts).
func rowOrPad(rows []string, index, padWidth int) string {
	if index >= len(rows) {
		return strings.Repeat(" ", padWidth)
	}
	return rows[index]
}
