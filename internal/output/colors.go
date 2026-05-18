package output

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ararahq/cli/internal/version"
)

// Brand palette — pulled verbatim from Identidade/ararahq-identidade.pen
// (see mcp__pencil__get_variables output committed in PR notes). When
// designers change a token, mirror it here. Names follow the .pen file
// so a single grep across repos finds every reference.
const (
	// Primary brand teal used in the logo, banner mascot, headers,
	// hyperlinks. Equivalent to .pen `brand-500`.
	brandColorTeal = "#1c99a7"
	// brandColorAccent is the soft turquoise used for hover/highlight
	// (mascot beak, link hover). .pen `brand-300` / `text-accent`.
	brandColorAccent = "#aff1f2"
	// Status semantic colours from the .pen file (success/warning/danger).
	colorSuccess = "#10b981"
	colorWarning = "#f59e0b"
	colorDanger  = "#ef4444"

	// Neutral text scale. .pen `text-secondary` / `text-tertiary` are the
	// two greys we surface; the rest stays as-is in lipgloss styles.
	colorTextSecondary = "#76a0a6"
	colorTextTertiary  = "#4b6a6d"
	colorTextPrimary   = "#ffffff"

	// Aliases retained for legacy call sites that reference these names.
	// New code should prefer the brand/text aliases above so a future
	// palette swap is a one-line change.
	colorGreen  = colorSuccess
	colorRed    = colorDanger
	colorYellow = colorWarning
	colorGray   = colorTextSecondary
	colorWhite  = colorTextPrimary

	// brandColorOrange is kept ONLY for the table headers / progress bars
	// that still reference it as a gradient anchor. Swapped to brand teal
	// so headers match the new identity. Legacy name preserved to avoid a
	// rename across the whole package; treat as deprecated.
	brandColorOrange = brandColorTeal

	envThemeName = "ARARA_THEME"
	envNoColor   = "NO_COLOR"
)

type ThemeName string

const (
	ThemeAuto ThemeName = "auto"
	ThemeMono ThemeName = "mono"
)

type Theme struct {
	Brand     lipgloss.Style
	Success   lipgloss.Style
	Error     lipgloss.Style
	Warning   lipgloss.Style
	Dim       lipgloss.Style
	Bold      lipgloss.Style
	Header    lipgloss.Style
	Subheader lipgloss.Style
}

var (
	themeMu     sync.RWMutex
	activeName  ThemeName
	autoOnce    sync.Once
	cachedAuto  *Theme
	cachedMono  *Theme
	monoBuildMu sync.Mutex
)

var (
	BrandStyle     lipgloss.Style
	SuccessStyle   lipgloss.Style
	ErrorStyle     lipgloss.Style
	WarningStyle   lipgloss.Style
	DimStyle       lipgloss.Style
	BoldStyle      lipgloss.Style
	HeaderStyle    lipgloss.Style
	SubheaderStyle lipgloss.Style
)

func init() {
	resolved := resolveInitialTheme()
	ApplyTheme(resolved)
}

func ApplyTheme(name ThemeName) {
	theme := buildTheme(name)

	themeMu.Lock()
	activeName = name
	BrandStyle = theme.Brand
	SuccessStyle = theme.Success
	ErrorStyle = theme.Error
	WarningStyle = theme.Warning
	DimStyle = theme.Dim
	BoldStyle = theme.Bold
	HeaderStyle = theme.Header
	SubheaderStyle = theme.Subheader
	themeMu.Unlock()

	applyGlobalColorProfile(name)
}

// applyGlobalColorProfile forces lipgloss to render plain text everywhere when
// the mono theme is active. This is what makes --theme=mono affect the REPL,
// dashboards, badges, and any other code that builds lipgloss styles directly
// (not just users of the package-level *Style vars).
func applyGlobalColorProfile(name ThemeName) {
	if name == ThemeMono {
		lipgloss.SetColorProfile(termenv.Ascii)
		return
	}
	lipgloss.SetColorProfile(termenv.EnvColorProfile())
}

func ActiveTheme() ThemeName {
	themeMu.RLock()
	defer themeMu.RUnlock()
	return activeName
}

func ParseThemeName(raw string) ThemeName {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case string(ThemeMono), "none", "off", "no":
		return ThemeMono
	default:
		return ThemeAuto
	}
}

func resolveInitialTheme() ThemeName {
	if value := strings.TrimSpace(os.Getenv(envNoColor)); value != "" {
		return ThemeMono
	}
	if value := strings.TrimSpace(os.Getenv(envThemeName)); value != "" {
		return ParseThemeName(value)
	}
	return ThemeAuto
}

func buildTheme(name ThemeName) *Theme {
	switch name {
	case ThemeMono:
		return monoTheme()
	default:
		return autoTheme()
	}
}

func autoTheme() *Theme {
	autoOnce.Do(func() {
		cachedAuto = &Theme{
			Brand:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandColorTeal)),
			Success:   lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)),
			Error:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorDanger)),
			Warning:   lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarning)),
			Dim:       lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextSecondary)),
			Bold:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorTextPrimary)),
			Header:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(brandColorTeal)),
			Subheader: lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextSecondary)),
		}
	})
	return cachedAuto
}

func monoTheme() *Theme {
	monoBuildMu.Lock()
	defer monoBuildMu.Unlock()

	if cachedMono != nil {
		return cachedMono
	}

	plain := lipgloss.NewStyle()
	cachedMono = &Theme{
		Brand:     plain,
		Success:   plain,
		Error:     plain,
		Warning:   plain,
		Dim:       plain,
		Bold:      plain,
		Header:    plain,
		Subheader: plain,
	}
	return cachedMono
}

func PrintHeader() {
	fmt.Println(RenderBanner(version.Version))
}
