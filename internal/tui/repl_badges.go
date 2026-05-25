package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared badge palette for "pill" cells inside tables.
//
// Strategy mirrors Stripe/Vercel CLIs: a single neutral chip background
// (bg-overlay / ink-700 from .pen) carries every status; the semantic
// hue lives ONLY in the foreground glyph + label. This keeps the visual
// rhythm of a column consistent (every pill has the same shape & weight)
// while letting colour do the semantic work — far less screamy than
// tinting the background per status, which fights with the result panel
// border and turns a dense table into a circus.
//
// Brand-tinted pills (sPillBrand) get the deep-teal fill that the LIVE
// header badge uses, reserved for "this is the row in focus" moments.
var pillBase = lipgloss.NewStyle().
	Bold(true).
	Background(lipgloss.Color(cFaint)).
	Padding(0, 1)

var (
	sPillSuccess = pillBase.Foreground(lipgloss.Color(cGreen))
	sPillWarn    = pillBase.Foreground(lipgloss.Color(cYellow))
	sPillDanger  = pillBase.Foreground(lipgloss.Color(cRed))
	sPillInfo    = pillBase.Foreground(lipgloss.Color(cBrandHi))
	sPillNeutral = pillBase.Foreground(lipgloss.Color(cMuted))
)

// templateStatusBadge maps a provider-status string to its semantic pill.
// The icons (✓ / ⧗ / ⏵ / ✘) are picked so the column scans as a vertical
// rhythm even before the user reads the words.
func templateStatusBadge(providerStatus string) string {
	switch strings.ToUpper(strings.TrimSpace(providerStatus)) {
	case "APPROVED":
		return sPillSuccess.Render("✓ APPROVED")
	case "PENDING", "SUBMITTED":
		return sPillWarn.Render("⧗ PENDING")
	case "RECEIVED":
		return sPillWarn.Render("⏵ RECEIVED")
	case "REJECTED", "FAILED":
		return sPillDanger.Render("✘ REJECTED")
	case "PAUSED", "DISABLED":
		return sPillNeutral.Render("◌ PAUSED")
	default:
		if providerStatus == "" {
			return sPillNeutral.Render("◌ UNKNOWN")
		}
		return sPillNeutral.Render("◌ " + strings.ToUpper(providerStatus))
	}
}

// campaignStatusBadge keeps campaigns in their own colour palette so a
// dense list of campaigns and templates side by side stays distinct.
func campaignStatusBadge(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "DONE":
		return sPillSuccess.Render("✓ COMPLETED")
	case "RUNNING", "PROCESSING", "SENDING":
		return sPillInfo.Render("◉ RUNNING")
	case "INGESTING", "QUEUED", "SCHEDULED":
		return sPillWarn.Render("⧗ " + strings.ToUpper(strings.TrimSpace(status)))
	case "CANCELED", "CANCELLED":
		return sPillNeutral.Render("◌ CANCELED")
	case "FAILED":
		return sPillDanger.Render("✘ FAILED")
	default:
		return sPillNeutral.Render("◌ " + strings.ToUpper(strings.TrimSpace(status)))
	}
}

// apiKeyModeBadge styles the LIVE/TEST chip used in tables. We reuse the
// header-bar palette (sLiveBadge / sTestBadge) so the same chip looks the
// same wherever it shows up.
func apiKeyModeBadge(mode string) string {
	if strings.EqualFold(mode, "live") {
		return sLiveBadge.Render(" LIVE ")
	}
	return sTestBadge.Render(" TEST ")
}

// categoryGlyph returns a one-character icon for a template category so
// the "what kind of message is this" column reads at a glance. The chars
// are picked from the same Unicode block to keep visual weight even.
func categoryGlyph(category string) string {
	switch strings.ToUpper(strings.TrimSpace(category)) {
	case "MARKETING":
		return "▸"
	case "AUTHENTICATION":
		return "⚿"
	case "UTILITY":
		return "◆"
	default:
		return "·"
	}
}

// categoryColor returns the foreground colour the category glyph + label
// should render in. Pure .pen brand-scale (no off-palette pinks/blues) so
// every category stays inside the AraraHQ identity — the differentiation
// comes from the glyph + the position in the teal ladder.
func categoryColor(category string) string {
	switch strings.ToUpper(strings.TrimSpace(category)) {
	case "MARKETING":
		return cWarning // amber — marketing reads as "attention-pulling"
	case "AUTHENTICATION":
		return cBrand // logo teal — security/identity cue
	case "UTILITY":
		return cMuted // .pen text-secondary — passive notification
	default:
		return cDim
	}
}

// cWarning is an alias to keep this file readable when picking semantic
// colours — `cYellow` reads as "yellow" (which it is) but in usage we
// mean "warning/attention", which categoryColor relies on.
const cWarning = cYellow

// formatCategoryCell composes the "▸ MARKETING" or "⚿ AUTHENTICATION"
// cell with the right glyph + colour pair so each table call site stays
// a one-liner.
func formatCategoryCell(category string) string {
	if category == "" {
		return sDim.Render("·")
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(categoryColor(category)))
	upper := strings.ToUpper(strings.TrimSpace(category))
	return style.Render(categoryGlyph(category) + " " + upper)
}
