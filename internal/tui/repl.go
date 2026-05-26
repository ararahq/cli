package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/commands"
	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/version"
)

const (
	replDocsURL        = "https://docs.ararahq.com"
	replPlatformMac    = "darwin"
	replPlatformLinux  = "linux"
	replOpenCmdMac     = "open"
	replOpenCmdLinux   = "xdg-open"
	replMaxOutputLines = 500
	replPromptSymbol   = "❯ "
)

// ── Palette ──────────────────────────────────────────────────────
//
// Mirrors Identidade/ararahq-identidade.pen tokens. Non-brand semantic
// colours (success/warning/danger) come from the same .pen file so a
// designer change propagates through one grep. Names kept short (cBrand,
// cSurface) to avoid noise — the comments map back to .pen tokens.
const (
	// Brand teal scale — single hue, 9 stops. Use cBrand for ink/links, the
	// lighter cBrandHi tones for hover / accent text, the darker cBrandDeep
	// for filled backgrounds (live badge, focus highlights).
	cBrand     = "#1c99a7" // brand-500 (logo)
	cBrandHi   = "#38d1d8" // brand-400 (hover / interactive accent)
	cBrandSoft = "#77e7e9" // brand-300 (active text, category cues)
	cAccent    = "#aff1f2" // brand-200 / text-accent (callout)
	cBrandDeep = "#205f6a" // brand-700 (filled chip backgrounds)
	cBrandDark = "#0f343d" // brand-800 (deepest fill, header strokes)

	// Semantic — same hex as .pen success / warning / danger / info.
	cGreen  = "#10b981" // success
	cYellow = "#f59e0b" // warning
	cRed    = "#ef4444" // danger
	cBlue   = cBrand    // info ≡ brand-500 per .pen

	// Ink scale — text + borders + neutral chip fills, darkest at the bottom.
	cWhite   = "#ffffff" // text-primary / paper-1000
	cMuted   = "#76a0a6" // text-secondary / ink-400
	cDim     = "#4b6a6d" // text-tertiary / ink-500
	cBorder  = "#37494a" // border-default / ink-600
	cFaint   = "#242e2f" // border-subtle / bg-overlay (neutral chip bg)
	cSurface = "#191f1f" // bg-elevated / ink-800 (zebra stripe)
	cInk     = "#122021" // bg-surface / ink-900
)

// ── Styles ───────────────────────────────────────────────────────

var (
	sBrand   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cBrand))
	sSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color(cGreen))
	sError   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cRed))
	sWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color(cYellow))
	sDim     = lipgloss.NewStyle().Foreground(lipgloss.Color(cDim))
	sMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color(cMuted))
	sBold    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cWhite))
	sBlue    = lipgloss.NewStyle().Foreground(lipgloss.Color(cBlue))
	sValue   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cBrand))

	sCard = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(cBorder)).
		Padding(1, 2)

	sCardHighlight = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(cBrand)).
			Padding(1, 2)

	// Live badge: deep teal background + bright accent foreground reads
	// as "production" without recycling the success-green palette.
	sLiveBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(cAccent)).
			Background(lipgloss.Color(cBrandDeep)).
			Padding(0, 1)

	// Test badge: warm amber on a muted background, distinct from live
	// teal so the eye picks up env mismatch immediately.
	sTestBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(cYellow)).
			Background(lipgloss.Color("#4A3800")).
			Padding(0, 1)
)

// ── Messages ─────────────────────────────────────────────────────

// commandResultMsg carries the rendered output back to the Update loop.
// The optional title/duration drive the result-panel header ("Templates ·
// 25 results · 124ms") so every async command gets a consistent visual
// receipt. command echoes the verb the user typed so the panel still
// reads naturally when the dispatcher renamed/normalized it.
type commandResultMsg struct {
	output   string
	title    string
	command  string
	duration time.Duration
	// raw skips the panel wrapper entirely — used for outputs that come
	// pre-styled with their own card (whoami, balance, send confirmation)
	// and shouldn't be double-bordered.
	raw bool
	// browsable, when non-nil, puts the REPL into drill-down mode after
	// the result is appended: arrow keys move a cursor over the rows,
	// Enter fetches the detail of the highlighted row, Esc closes. The
	// command field on the underlying result is reused so the prompt
	// stays consistent with the breadcrumb shown in the result panel.
	browsable *browsableResult
}

// drillDetailMsg comes back from a detail fetcher (e.g. GetCampaign) and
// is consumed by the Update loop to populate drill.detailBody. Kept as a
// distinct message so the spinner can stay on during the fetch — we
// don't want the user to wonder whether Enter did anything.
type drillDetailMsg struct {
	body string
	err  string
}

type commandErrorMsg struct {
	errorText string
	command   string
	duration  time.Duration
}

type initClientMsg struct {
	client      *api.Client
	profileName string
	mode        string
}

// timed wraps a tea.Cmd factory that returns a commandResultMsg /
// commandErrorMsg and stamps the duration + command-name on the result.
// Centralised here so every async handler gets the same "✓ 25 results ·
// 124ms" footer for free without each one having to remember to call
// time.Since.
func timed(commandName string, fn func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		msg := fn()
		elapsed := time.Since(start)
		switch typed := msg.(type) {
		case commandResultMsg:
			if typed.command == "" {
				typed.command = commandName
			}
			typed.duration = elapsed
			return typed
		case commandErrorMsg:
			if typed.command == "" {
				typed.command = commandName
			}
			typed.duration = elapsed
			return typed
		default:
			return msg
		}
	}
}

// ── Model ────────────────────────────────────────────────────────

type REPLModel struct {
	input          textinput.Model
	spinner        spinner.Model
	outputLines    []string
	apiClient      *api.Client
	profileName    string
	mode           string
	width          int
	height         int
	executing      bool
	ready          bool
	placeholderIdx int
	// hasInteracted flips to true the first time the user runs a real
	// command. The welcome banner collapses to a one-liner header from
	// that point on so the result area gets the screen real estate it
	// needs instead of being squeezed under a 6-line splash forever.
	hasInteracted bool
	// runningCommand is the verb of the in-flight async command (e.g.
	// "templates") — surfaced in the spinner line so the user knows
	// exactly what is taking time, not just "running...".
	runningCommand string
	// runningSince stamps when the current spinner started, used to
	// reveal elapsed time after a few hundred ms (so quick commands
	// don't flash a "0ms" counter that adds noise).
	runningSince time.Time
	// welcomeBanner holds the splash content shown on first connect.
	// Stored separately from outputLines so renderOutputArea can collapse
	// it the moment hasInteracted flips, without rewriting the line buffer.
	welcomeBanner string
	// drill is the currently-active browsable result, if any. nil when
	// the most recent command didn't produce a navigable table — the
	// REPL falls back to the plain prompt experience in that case.
	drill *drillState
}

func NewREPLModel() REPLModel {
	inputField := textinput.New()
	inputField.Placeholder = placeholderRotation[0]
	inputField.Focus()
	inputField.CharLimit = 500
	inputField.Prompt = ""
	inputField.PlaceholderStyle = sDim

	// Inline tab completion: as the user types, bubbles renders a greyed-out
	// suggestion after the cursor; pressing Tab accepts it. Same UX as
	// fish shell, gh, and Claude Code.
	inputField.SetSuggestions(completionSuggestions)
	inputField.ShowSuggestions = true

	loadSpinner := spinner.New()
	loadSpinner.Spinner = spinner.Dot
	loadSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(cBrand))

	return REPLModel{
		input:       inputField,
		spinner:     loadSpinner,
		outputLines: []string{},
	}
}

func (repl REPLModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		repl.spinner.Tick,
		repl.initializeClient(),
		placeholderTickCmd(),
	)
}

func (repl REPLModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMessage := message.(type) {
	case tea.KeyMsg:
		return repl.handleKeyPress(typedMessage)
	case tea.WindowSizeMsg:
		repl.width = typedMessage.Width
		repl.height = typedMessage.Height
		inputWidth := typedMessage.Width - 6
		if inputWidth < 20 {
			inputWidth = 20
		}
		repl.input.Width = inputWidth
		return repl, nil
	case commandResultMsg:
		repl.executing = false
		repl.runningCommand = ""
		repl.hasInteracted = true
		// Browsable results take the drill path: the result body is
		// rendered live (inside View()) instead of being baked into the
		// scrollback, so arrow keys can redraw the cursor without the
		// terminal losing previous state.
		if typedMessage.browsable != nil {
			repl.drill = &drillState{result: *typedMessage.browsable}
			// Still echo a small header into the scrollback so the user
			// has a permanent trail of what they ran.
			repl.appendOutput(sDim.Render("  ") + sBrand.Render("▾ ") + sMuted.Render(typedMessage.title))
			return repl, nil
		}
		if typedMessage.raw || typedMessage.title == "" {
			repl.appendOutput(typedMessage.output)
		} else {
			repl.appendOutput(renderResultPanel(
				typedMessage.title,
				typedMessage.command,
				typedMessage.duration,
				typedMessage.output,
				repl.width,
			))
		}
		return repl, nil
	case drillDetailMsg:
		repl.executing = false
		repl.runningCommand = ""
		if repl.drill == nil {
			return repl, nil
		}
		repl.drill.detailBody = typedMessage.body
		repl.drill.detailErr = typedMessage.err
		return repl, nil
	case commandErrorMsg:
		repl.executing = false
		repl.runningCommand = ""
		repl.hasInteracted = true
		repl.appendOutput(renderResultErrorPanel(
			typedMessage.command,
			typedMessage.duration,
			typedMessage.errorText,
			repl.width,
		))
		return repl, nil
	case initClientMsg:
		repl.apiClient = typedMessage.client
		repl.profileName = typedMessage.profileName
		repl.mode = typedMessage.mode
		repl.ready = true
		// Welcome banner only on first connect. Once the user starts
		// running commands, the renderHeaderBar at the top carries the
		// same identity (profile + mode + version) in one line, so
		// re-rendering the full banner above every result would be
		// noise. Stored on the model — not in outputLines — so the
		// view can drop it the instant hasInteracted flips without
		// hunting through the buffer.
		repl.welcomeBanner = renderWelcomeBanner(typedMessage.profileName, typedMessage.mode)
		return repl, nil
	case spinner.TickMsg:
		updatedSpinner, spinnerCmd := repl.spinner.Update(message)
		repl.spinner = updatedSpinner
		return repl, spinnerCmd
	case placeholderTickMsg:
		// Rotate the empty-input hint so the prompt feels alive even when
		// the user is just sitting on the welcome screen reading. We only
		// flip the placeholder when the field is actually empty so a
		// half-typed command doesn't have its hint yanked mid-sentence.
		repl.placeholderIdx = (repl.placeholderIdx + 1) % len(placeholderRotation)
		if repl.input.Value() == "" {
			repl.input.Placeholder = placeholderRotation[repl.placeholderIdx]
		}
		return repl, placeholderTickCmd()
	default:
		updatedInput, inputCommand := repl.input.Update(message)
		repl.input = updatedInput
		return repl, inputCommand
	}
}

func (repl REPLModel) View() string {
	if repl.width == 0 {
		return ""
	}

	var sections []string

	// header bar
	sections = append(sections, repl.renderHeaderBar())
	sections = append(sections, "")

	// output area
	outputArea := repl.renderOutputArea()
	if outputArea != "" {
		sections = append(sections, outputArea)
	}

	// drill panel — live (re-rendered every keystroke so the cursor
	// updates instantly). Sits between the scrollback and the prompt so
	// the input box stays anchored at the bottom.
	if repl.drill != nil {
		sections = append(sections, repl.renderDrillArea())
	}

	// prompt
	sections = append(sections, repl.renderPromptLine())

	// inline hint — shown only when the user has typed a partial command
	// the dispatcher would recognize after one more keystroke. Keeps the
	// REPL feeling responsive ("the CLI knows what I'm doing") without
	// being noisy when the input is empty or already canonical.
	if hint := repl.renderInlineHint(); hint != "" {
		sections = append(sections, hint)
	}

	sections = append(sections, "")

	// footer hints
	sections = append(sections, repl.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// ── Header ───────────────────────────────────────────────────────

func (repl REPLModel) renderHeaderBar() string {
	logoStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cBrand))
	sep := sDim.Render(" · ")

	left := logoStyle.Render("◆ arara")

	if repl.profileName != "" {
		left += sep + sBold.Render(repl.profileName) + sep + renderModeBadge(repl.mode)
	}

	versionText := sMuted.Render(fmt.Sprintf("v%s", version.Version))

	padding := repl.width - lipgloss.Width(left) - lipgloss.Width(versionText) - 2
	if padding < 1 {
		padding = 1
	}

	line := left + strings.Repeat(" ", padding) + versionText

	borderStyle := lipgloss.NewStyle().
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(cFaint)).
		Width(repl.width)

	return borderStyle.Render(line)
}

func renderModeBadge(mode string) string {
	if mode == "LIVE" {
		return sLiveBadge.Render(" LIVE ")
	}
	return sTestBadge.Render(" TEST ")
}

func replKeyName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "cli"
	}
	return "cli-" + hostname
}

// ── Output Area ──────────────────────────────────────────────────

func (repl *REPLModel) appendOutput(text string) {
	lines := strings.Split(text, "\n")
	repl.outputLines = append(repl.outputLines, lines...)

	if len(repl.outputLines) > replMaxOutputLines {
		excess := len(repl.outputLines) - replMaxOutputLines
		repl.outputLines = repl.outputLines[excess:]
	}
}

func (repl REPLModel) renderOutputArea() string {
	visibleHeight := repl.height - 8
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	// Compose the visible buffer. Pre-interaction we show the welcome
	// banner alone; post-interaction we drop it and let the result
	// panels fill the screen — the renderHeaderBar at top still carries
	// org/profile/mode so identity is never lost.
	if !repl.hasInteracted {
		if repl.welcomeBanner == "" && len(repl.outputLines) == 0 {
			return ""
		}
		var combined []string
		if repl.welcomeBanner != "" {
			combined = append(combined, repl.welcomeBanner)
		}
		combined = append(combined, repl.outputLines...)
		return tailLines(strings.Join(combined, "\n"), visibleHeight)
	}

	if len(repl.outputLines) == 0 {
		return ""
	}
	return tailLines(strings.Join(repl.outputLines, "\n"), visibleHeight)
}

// renderDrillArea composes the live drill UI: either the list with a
// cursor row + footer, or the list collapsed to a single breadcrumb
// followed by the detail panel + footer. Re-evaluated on every key event
// so the cursor stripe redraws without explicit invalidation.
func (repl REPLModel) renderDrillArea() string {
	if repl.drill == nil {
		return ""
	}
	var lines []string

	// Panel header (same accent bar idiom as renderResultPanel) so the
	// drill feels like an extension of the just-returned result.
	header := buildPanelHeading(repl.drill.result.title, repl.drill.result.command, 0)
	lines = append(lines, "")
	lines = append(lines, "  "+header)

	if !repl.drill.viewingItem {
		lines = append(lines, "")
		lines = append(lines, renderBrowsableTable(*repl.drill))
		lines = append(lines, "")
		lines = append(lines, renderDrillFooter(*repl.drill))
		return strings.Join(lines, "\n")
	}

	// In detail mode we still echo the currently-selected row at the
	// top so the user remembers what they drilled into without
	// scrolling — analogous to a breadcrumb.
	selected := repl.drill.result.rows[repl.drill.cursor]
	lines = append(lines, "")
	lines = append(lines, "  "+sBrand.Render("→ ")+strings.Join(selected.cells, "  "))
	lines = append(lines, "")
	lines = append(lines, renderDrillDetail(*repl.drill, repl.width))
	lines = append(lines, "")
	lines = append(lines, renderDrillFooter(*repl.drill))
	return strings.Join(lines, "\n")
}

// tailLines keeps the last n lines of a multi-line block — the scrollback
// behaviour the REPL has always had, factored out so both the welcome and
// post-interaction paths use the same windowing logic.
func tailLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// ── Prompt ───────────────────────────────────────────────────────

func (repl REPLModel) renderPromptLine() string {
	promptStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cBrand))

	if repl.executing {
		label := "working"
		if repl.runningCommand != "" {
			label = repl.runningCommand
		}
		line := "  " + repl.spinner.View() + " " + sBrand.Render(label) + sDim.Render(" …")
		if !repl.runningSince.IsZero() {
			elapsed := time.Since(repl.runningSince)
			if elapsed > 600*time.Millisecond {
				line += sDim.Render("  " + formatElapsed(elapsed))
			}
		}
		return line
	}

	return promptStyle.Render(replPromptSymbol) + repl.input.View()
}

// renderInlineHint shows "↳ runs <canonical>" below the prompt when the
// typed prefix unambiguously points at a known command. Returns empty
// when the input is empty, executing, or already canonical.
func (repl REPLModel) renderInlineHint() string {
	if repl.executing {
		return ""
	}
	hint := inputHintFor(repl.input.Value())
	if hint == "" {
		return ""
	}
	return sDim.Render("    ↳ ") + sBlue.Render(strings.TrimSpace(hint))
}

func (repl REPLModel) renderFooter() string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cMuted))

	hints := []string{
		keyStyle.Render("/help") + sDim.Render(" commands"),
		keyStyle.Render("/send") + sDim.Render(" message"),
		keyStyle.Render("esc") + sDim.Render(" quit"),
	}

	return sDim.Render("  ") + strings.Join(hints, sDim.Render("  ·  "))
}

// ── Welcome ──────────────────────────────────────────────────────

func renderWelcomeBanner(profileName string, mode string) string {
	// The shared output.RenderBanner gives us the canonical mascot +
	// wordmark + subtitle + version block. The REPL just adds the
	// connection status + suggestion hints below.
	banner := output.RenderBanner(version.Version)

	statusLine := sSuccess.Render("  ✔ Connected") +
		sDim.Render(" as ") + sBold.Render(profileName) +
		sDim.Render(" · ") + renderModeBadge(mode)

	hintLine := sDim.Render("  Try: ") +
		sBlue.Render("templates") + sDim.Render(", ") +
		sBlue.Render("status") + sDim.Render(", ") +
		sBlue.Render("send +55... Hello!") + sDim.Render(", or ") +
		sBlue.Render("/help")

	return "\n" + banner + "\n" + statusLine + "\n" + hintLine + "\n"
}

// ── Key Handling ─────────────────────────────────────────────────

func (repl REPLModel) handleKeyPress(keyMessage tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Browse-mode key handling takes priority: when a navigable result is
	// on screen, ↑/↓/Enter/Esc operate on the cursor instead of the
	// input field, otherwise typing-while-browsing would silently steal
	// focus and the user would type characters into a hidden buffer.
	if repl.drill != nil {
		if model, cmd, handled := repl.handleBrowseKey(keyMessage); handled {
			return model, cmd
		}
	}

	switch keyMessage.Type {
	case tea.KeyCtrlC:
		return repl, tea.Quit
	case tea.KeyEsc:
		// Esc never quits when there's a drill active — already handled
		// above. Without a drill, Esc clears any in-progress input but
		// only quits when the input is already empty (avoid losing a
		// half-typed command to a stray Esc).
		if repl.input.Value() != "" {
			repl.input.SetValue("")
			return repl, nil
		}
		return repl, tea.Quit
	case tea.KeyEnter:
		return repl.handleCommandSubmit()
	default:
		updatedInput, inputCommand := repl.input.Update(keyMessage)
		repl.input = updatedInput
		return repl, inputCommand
	}
}

// handleBrowseKey processes arrow / Enter / Esc when a browsable result
// is active. Returns handled=false when the key isn't a browse key so the
// caller falls through to normal input handling — this is how characters
// still get typed when the user wants to fire a new command while a
// table is on screen (q to close, then type freely).
func (repl REPLModel) handleBrowseKey(keyMessage tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch keyMessage.Type {
	case tea.KeyUp:
		if !repl.drill.viewingItem && repl.drill.cursor > 0 {
			repl.drill.cursor--
		}
		return repl, nil, true
	case tea.KeyDown:
		if !repl.drill.viewingItem && repl.drill.cursor < len(repl.drill.result.rows)-1 {
			repl.drill.cursor++
		}
		return repl, nil, true
	case tea.KeyEnter:
		if repl.drill.viewingItem {
			// Already in detail view — pressing Enter again does nothing.
			return repl, nil, true
		}
		return repl.startDrillDetail()
	case tea.KeyEsc:
		if repl.drill.viewingItem {
			// Drop the detail body, return to the list with cursor
			// preserved.
			repl.drill.viewingItem = false
			repl.drill.detailBody = ""
			repl.drill.detailErr = ""
			return repl, nil, true
		}
		// On the list view, Esc closes the drill entirely so the user
		// can run a fresh command without first having to "press q".
		repl.drill = nil
		return repl, nil, true
	case tea.KeyRunes:
		// `q` is the universal "close browser" key in k9s / less / man.
		if len(keyMessage.Runes) == 1 && keyMessage.Runes[0] == 'q' && repl.input.Value() == "" {
			repl.drill = nil
			return repl, nil, true
		}
	}
	return repl, nil, false
}

// startDrillDetail kicks off the per-row detail fetch. We flip
// viewingItem on immediately so the loading spinner renders against the
// detail layout (no jarring layout shift after the result arrives), and
// spin a tea.Cmd to actually call the fetcher.
func (repl REPLModel) startDrillDetail() (tea.Model, tea.Cmd, bool) {
	if repl.drill == nil || len(repl.drill.result.rows) == 0 {
		return repl, nil, true
	}
	if repl.drill.result.detailFunc == nil {
		// No detail fetcher wired — happens for tables where there's no
		// meaningful per-row view (e.g. phone numbers). We surface a
		// short hint instead of failing silently.
		repl.drill.viewingItem = true
		repl.drill.detailBody = sDim.Render("  (no detail view available for this list)")
		return repl, nil, true
	}
	id := repl.drill.result.rows[repl.drill.cursor].id
	repl.drill.viewingItem = true
	repl.drill.detailBody = sDim.Render("  loading…")
	repl.drill.detailErr = ""
	repl.executing = true
	repl.runningCommand = "detail"
	repl.runningSince = time.Now()

	fetch := repl.drill.result.detailFunc
	cmd := func() tea.Msg {
		body, err := fetch(id)
		if err != nil {
			return drillDetailMsg{err: err.Error()}
		}
		return drillDetailMsg{body: body}
	}
	return repl, cmd, true
}

func (repl REPLModel) handleCommandSubmit() (tea.Model, tea.Cmd) {
	rawInput := repl.input.Value()
	action := ParseInput(rawInput)
	if action.Kind == ActionNoop {
		return repl, nil
	}

	// Echo the (trimmed) command so the user sees what's running.
	promptEcho := sBrand.Render(replPromptSymbol) + sMuted.Render(strings.TrimSpace(rawInput))
	repl.appendOutput(promptEcho)
	repl.input.SetValue("")

	if action.RequiresAPIClient && repl.apiClient == nil {
		repl.appendOutput(renderNotLoggedIn())
		return repl, nil
	}

	return repl.dispatchAction(action)
}

// runAsync flips the spinner on, stamps "what is running" so the busy
// state can show a human label, and wraps the underlying tea.Cmd so the
// returned message carries a duration. Returning the model+cmd as a
// tuple keeps the call sites in dispatchAction one-liners.
func (repl REPLModel) runAsync(commandName string, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	repl.executing = true
	repl.runningCommand = commandName
	repl.runningSince = time.Now()
	return repl, timed(commandName, func() tea.Msg {
		// cmd is a tea.Cmd (func() tea.Msg). Calling it executes the
		// underlying handler and produces the result message; timed() then
		// wraps that message with the elapsed duration.
		return cmd()
	})
}

func (repl REPLModel) dispatchAction(action Action) (tea.Model, tea.Cmd) {
	switch action.Kind {
	case ActionQuit:
		return repl, tea.Quit
	case ActionShowHelp:
		repl.appendOutput(renderHelpText())
		return repl, nil
	case ActionClearOutput:
		repl.outputLines = []string{}
		return repl, nil
	case ActionShowWhoami:
		repl.appendOutput(repl.renderWhoami())
		return repl, nil
	case ActionOpenDocs:
		repl.appendOutput(openDocsInBrowser())
		return repl, nil
	case ActionShowConfig:
		repl.appendOutput(repl.renderConfig())
		return repl, nil
	case ActionShowListenHint:
		repl.appendOutput(renderListenHint())
		return repl, nil
	case ActionRunSend:
		return repl.runAsync("send", repl.executeSendCommand(action.Args))
	case ActionRunTemplates:
		return repl.runAsync("templates", repl.executeTemplatesCommand())
	case ActionRunStatus:
		return repl.runAsync("status", repl.executeStatusCommand())
	case ActionRunKeys:
		return repl.runAsync("keys", repl.executeKeysCommand(action.Args))
	case ActionRunLogs:
		return repl.runAsync("logs", repl.executeLogsCommand())
	case ActionRunCampaigns:
		return repl.runAsync("campaigns", repl.executeCampaignsCommand(action.Args))
	case ActionRunContacts:
		return repl.runAsync("contacts", repl.executeContactsCommand(action.Args))
	case ActionRunNumbers:
		return repl.runAsync("numbers", repl.executeNumbersCommand())
	case ActionRunEstimate:
		return repl.runAsync("estimate", repl.executeEstimateCommand(action.Args))
	case ActionRunBalance:
		return repl.runAsync("balance", repl.executeBalanceCommand())
	case ActionUnknown:
		return repl.dispatchSlashOrUnknown(action)
	default:
		return repl, nil
	}
}

func (repl REPLModel) dispatchSlashOrUnknown(action Action) (tea.Model, tea.Cmd) {
	candidate := SlashCommandName(action.Command)

	loader := commands.NewLoader("")
	slashCommand, lookupErr := loader.Lookup(candidate)
	if lookupErr != nil {
		repl.appendOutput(sError.Render(fmt.Sprintf("  unknown: %s", action.Command)))
		repl.appendOutput(sDim.Render("  type /help for available commands"))
		return repl, nil
	}

	repl.executing = true
	return repl, executeSlashCommand(*slashCommand, action.Args)
}

func executeSlashCommand(command commands.Command, rawArgs string) tea.Cmd {
	return func() tea.Msg {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		args := splitSlashCommandArgs(rawArgs)

		_, runErr := commands.Execute(context.Background(), command, commands.ExecOptions{
			Args:   args,
			Stdout: stdout,
			Stderr: stderr,
		})

		output := renderSlashCommandOutput(command, stdout.String(), stderr.String(), runErr)
		if runErr != nil {
			return commandResultMsg{output: output}
		}
		return commandResultMsg{output: output}
	}
}

func splitSlashCommandArgs(rawArgs string) []string {
	trimmed := strings.TrimSpace(rawArgs)
	if trimmed == "" {
		return nil
	}
	// Simple whitespace split — quoted args are not supported in v1; users
	// who need them can write a more elaborate command body and parse
	// $@ themselves.
	return strings.Fields(trimmed)
}

func renderSlashCommandOutput(command commands.Command, stdout, stderr string, runErr error) string {
	var builder strings.Builder

	builder.WriteString(sDim.Render(fmt.Sprintf("  ⌁ %s (%s)\n", command.Name, command.Source)))

	stdoutTrim := strings.TrimRight(stdout, "\n")
	if stdoutTrim != "" {
		for _, line := range strings.Split(stdoutTrim, "\n") {
			builder.WriteString("  ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	stderrTrim := strings.TrimRight(stderr, "\n")
	if stderrTrim != "" {
		for _, line := range strings.Split(stderrTrim, "\n") {
			builder.WriteString("  ")
			builder.WriteString(sWarn.Render(line))
			builder.WriteString("\n")
		}
	}

	if runErr != nil {
		builder.WriteString(sError.Render("  ✘ " + runErr.Error() + "\n"))
	}

	return strings.TrimRight(builder.String(), "\n")
}

// ── Init Client ──────────────────────────────────────────────────

func (repl REPLModel) initializeClient() tea.Cmd {
	return func() tea.Msg {
		apiClient, clientError := api.NewClientFromConfig()
		if clientError != nil {
			return commandErrorMsg{errorText: "Not logged in — run: arara login --key <your-key>"}
		}

		configuration, configError := config.Load()
		if configError != nil {
			return commandErrorMsg{errorText: "Failed to load config"}
		}

		return initClientMsg{
			client:      apiClient,
			profileName: configuration.CurrentProfile,
			mode:        apiClient.Mode(),
		}
	}
}

// ── /help ────────────────────────────────────────────────────────

func renderHelpText() string {
	sectionSep := func(label string) string {
		lineWidth := 44
		labelLen := len(label) + 2
		remaining := lineWidth - labelLen
		if remaining < 2 {
			remaining = 2
		}
		return sDim.Render("  -- " + label + " " + strings.Repeat("-", remaining))
	}

	cmd := func(name string, desc string) string {
		nameFormatted := sValue.Render(fmt.Sprintf("  %-28s", name))
		return nameFormatted + sMuted.Render(desc)
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, sectionSep("Messaging"))
	lines = append(lines, cmd("send <phone> -t <template>", "Send via template"))
	lines = append(lines, cmd("send <phone> -t <tpl> -v a,b", "Template with variables"))
	lines = append(lines, cmd("send <phone> <text>", "Freeform (open window only)"))
	lines = append(lines, cmd("templates", "List all templates"))
	lines = append(lines, cmd("logs", "Recent messages"))
	lines = append(lines, "")
	lines = append(lines, sectionSep("Campaigns"))
	lines = append(lines, cmd("campaigns", "List all campaigns"))
	lines = append(lines, cmd("campaigns <id>", "Campaign details"))
	lines = append(lines, cmd("estimate <template> <count>", "Estimate campaign cost"))
	lines = append(lines, "")
	lines = append(lines, sectionSep("Contacts"))
	lines = append(lines, cmd("contacts", "List contacts"))
	lines = append(lines, cmd("contacts search <query>", "Search contacts"))
	lines = append(lines, cmd("contacts stats", "Contact statistics"))
	lines = append(lines, "")
	lines = append(lines, sectionSep("Account"))
	lines = append(lines, cmd("status", "Credits, delivery rate, stats"))
	lines = append(lines, cmd("balance", "Wallet balance"))
	lines = append(lines, cmd("keys", "List API keys"))
	lines = append(lines, cmd("keys create <mode>", "Create new API key"))
	lines = append(lines, cmd("numbers", "Registered phone numbers"))
	lines = append(lines, cmd("whoami", "Current profile and mode"))
	lines = append(lines, "")
	lines = append(lines, sectionSep("Realtime"))
	lines = append(lines, cmd("listen", "Stream webhook events (SSE)"))
	lines = append(lines, "")
	lines = append(lines, sectionSep("Tools"))
	lines = append(lines, cmd("config", "Show configuration"))
	lines = append(lines, cmd("docs", "Open docs in browser"))
	lines = append(lines, cmd("clear", "Clear screen"))
	lines = append(lines, cmd("quit", "Exit"))
	lines = append(lines, "")

	return strings.Join(lines, "\n")
}

// ── /send ────────────────────────────────────────────────────────

func (repl REPLModel) executeSendCommand(arguments string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(arguments) == "" {
			return commandResultMsg{output: renderSendUsage()}
		}

		// Parse flags: send <phone> -t <template> [-v var1,var2]
		// or:          send <phone> <text>  (freeform, only works with open conversation window)
		phone, templateName, variables, freeformText := parseSendArgs(arguments)

		if phone == "" {
			return commandResultMsg{output: renderSendUsage()}
		}

		request := api.SendMessageRequest{
			Receiver: phone,
		}

		if templateName != "" {
			request.TemplateName = templateName
			request.TemplateVariables = variables
		} else if freeformText != "" {
			request.Body = freeformText
		} else {
			return commandResultMsg{output: renderSendUsage()}
		}

		response, sendError := repl.apiClient.SendMessage(request)
		if sendError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Send failed: %s", sendError.Error())}
		}

		card := sCard.BorderForeground(lipgloss.Color(cGreen)).Width(55)

		content := sSuccess.Render("✔ Message sent!") + "\n\n" +
			sMuted.Render("  To:       ") + sBold.Render(response.Receiver)

		if templateName != "" {
			content += "\n" + sMuted.Render("  Template: ") + sValue.Render(templateName)
			if len(variables) > 0 {
				content += "\n" + sMuted.Render("  Vars:     ") + sDim.Render(strings.Join(variables, ", "))
			}
		} else {
			content += "\n" + sMuted.Render("  Text:     ") + lipgloss.NewStyle().Foreground(lipgloss.Color(cWhite)).Render(freeformText)
		}

		content += "\n" + sMuted.Render("  Status:   ") + sBlue.Render(response.Status)

		if response.ID != "" {
			content += "\n" + sMuted.Render("  ID:       ") + sDim.Render(response.ID)
		}

		return commandResultMsg{output: "\n" + card.Render(content) + "\n"}
	}
}

// parseSendArgs aceita duas sintaxes (compat com `arara send` shell + REPL):
//
//	send +5511999 -t welcome           (posicional + flags curtas)
//	send --to +5511999 --template w    (estilo CLI shell)
//	send +5511999 Hello there          (freeform — janela 24h obrigatória)
//
// Flag desconhecida vira erro de uso, não silenciosamente "freeform" — evita
// que `--to <phone>` (estilo CLI) seja interpretado como texto livre e mandado
// pra processSessionMessage no backend.
func parseSendArgs(arguments string) (phone string, templateName string, variables []string, freeformText string) {
	parts := strings.Fields(arguments)
	if len(parts) == 0 {
		return
	}

	remaining := parts
	if !strings.HasPrefix(parts[0], "-") {
		phone = parts[0]
		remaining = parts[1:]
	}

	freeformParts := make([]string, 0, len(remaining))
	for index := 0; index < len(remaining); index++ {
		token := remaining[index]
		switch token {
		case "--to":
			if index+1 < len(remaining) {
				index++
				phone = remaining[index]
			}
		case "-t", "--template":
			if index+1 < len(remaining) {
				index++
				templateName = remaining[index]
			}
		case "-v", "--vars":
			if index+1 < len(remaining) {
				index++
				variables = strings.Split(remaining[index], ",")
			}
		default:
			if strings.HasPrefix(token, "-") {
				// flag desconhecida — devolve como freeformText pra REPL
				// mostrar erro de uso; nunca tratar silenciosamente como texto.
				freeformText = ""
				freeformParts = nil
				phone = ""
				templateName = ""
				variables = nil
				return
			}
			freeformParts = append(freeformParts, token)
		}
	}

	if templateName == "" && len(freeformParts) > 0 {
		freeformText = strings.Join(freeformParts, " ")
	}
	return
}

func renderSendUsage() string {
	card := sCard.Width(60)

	content := sBold.Render("Send a Message") + "\n\n" +
		sMuted.Render("  Via template (recommended):") + "\n" +
		sValue.Render("    send +5511999999999 -t welcome_msg") + "\n" +
		sValue.Render("    send +5511999999999 -t order_confirm -v João,#1234") + "\n\n" +
		sMuted.Render("  Freeform (requires open conversation window):") + "\n" +
		sValue.Render("    send +5511999999999 Hello from Arara!") + "\n\n" +
		sDim.Render("  Flags:") + "\n" +
		sDim.Render("    -t, --template <name>    Template name") + "\n" +
		sDim.Render("    -v, --vars <v1,v2,...>    Template variables")

	return "\n" + card.Render(content) + "\n"
}

// ── /templates ───────────────────────────────────────────────────

func (repl REPLModel) executeTemplatesCommand() tea.Cmd {
	return func() tea.Msg {
		templates, listError := repl.apiClient.ListTemplates()
		if listError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed to list templates: %s", listError.Error())}
		}
		if len(templates) == 0 {
			return commandResultMsg{
				title:  "Templates",
				output: sDim.Render("  No templates yet. Create one at ararahq.com → Templates."),
			}
		}
		// Build a browsable result so ↑↓Enter drill into each template's
		// body/category/status — a single command becomes a navigable
		// catalogue without forcing the user to type IDs.
		rows := make([]browsableRow, 0, len(templates))
		for _, template := range templates {
			rows = append(rows, browsableRow{
				id: template.Name,
				cells: []string{
					templateNameCell(template.Name),
					templateStatusBadge(template.ProviderStatus),
					formatCategoryCell(template.Category),
					sDim.Render(template.Language),
				},
			})
		}
		return commandResultMsg{
			title:   fmt.Sprintf("Templates · %d", len(templates)),
			command: "templates",
			browsable: &browsableResult{
				title:      fmt.Sprintf("Templates · %d", len(templates)),
				command:    "templates",
				columns:    []string{"NAME", "STATUS", "CATEGORY", "LANG"},
				rows:       rows,
				detailFunc: repl.templateDetailFetcher(templates),
			},
		}
	}
}

// templateDetailFetcher returns a closure suitable for browsableResult.
// We snapshot the already-fetched templates slice so the drill detail
// doesn't have to round-trip just to render fields the list call already
// brought back. For templates that's enough; for campaigns/contacts the
// detail fetcher will need to actually call the API.
func (repl REPLModel) templateDetailFetcher(templates []api.Template) func(string) (string, error) {
	return func(id string) (string, error) {
		for _, tpl := range templates {
			if tpl.Name == id {
				return renderTemplateDetail(tpl), nil
			}
		}
		return "", fmt.Errorf("template %q not found", id)
	}
}

// renderTemplateDetail composes the drill-down view of a single
// template. Two columns by space alignment so it reads as a key/value
// sheet; body sits below in a quoted block so it's distinguishable from
// the metadata fields above it.
func renderTemplateDetail(tpl api.Template) string {
	var b strings.Builder
	b.WriteString(sMuted.Render("Name      ") + sBold.Render(tpl.Name) + "\n")
	b.WriteString(sMuted.Render("Category  ") + formatCategoryCell(tpl.Category) + "\n")
	b.WriteString(sMuted.Render("Status    ") + templateStatusBadge(tpl.ProviderStatus) + "\n")
	b.WriteString(sMuted.Render("Language  ") + sValue.Render(tpl.Language) + "\n")
	if tpl.Body != "" {
		b.WriteString("\n")
		b.WriteString(sDim.Render("BODY") + "\n")
		for _, line := range strings.Split(strings.TrimRight(tpl.Body, "\n"), "\n") {
			b.WriteString(sDim.Render("│ ") + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// templateNameCell renders the template name with a leading dot in the
// brand colour so each row gets a left-anchor that the eye uses to find
// the row beginning when scanning a long list.
func templateNameCell(name string) string {
	return sBrand.Render("• ") + sBold.Render(name)
}

// ── /status ──────────────────────────────────────────────────────

func (repl REPLModel) executeStatusCommand() tea.Cmd {
	return func() tea.Msg {
		mode := strings.ToLower(repl.mode)

		metrics, metricsError := repl.apiClient.GetMetrics(mode)
		if metricsError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed to fetch metrics: %s", metricsError.Error())}
		}

		walletBalance, walletError := repl.apiClient.GetWalletBalance(mode)
		if walletError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed to fetch wallet: %s", walletError.Error())}
		}

		balance := extractFloat(walletBalance, "balance")
		sent := extractFloat(metrics, "sent")
		delivered := extractFloat(metrics, "delivered")
		failed := extractFloat(metrics, "failed")

		deliveryRate := 0.0
		if sent > 0 {
			deliveryRate = (delivered / sent) * 100.0
		}

		// credit card
		creditCard := sCardHighlight.Width(22)
		creditContent := sMuted.Render("CREDITS") + "\n" +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cGreen)).Render(fmt.Sprintf("R$ %.2f", balance))

		// messages card
		msgCard := sCard.Width(22)
		msgContent := sMuted.Render("SENT") + "\n" +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cBlue)).Render(fmt.Sprintf("%.0f msgs", sent))

		// delivery card
		delCard := sCard.Width(22)
		var rateColor string
		if deliveryRate >= 95.0 {
			rateColor = cGreen
		} else if deliveryRate >= 80.0 {
			rateColor = cYellow
		} else {
			rateColor = cRed
		}
		delContent := sMuted.Render("DELIVERY") + "\n" +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(rateColor)).Render(fmt.Sprintf("%.1f%%", deliveryRate))

		cards := lipgloss.JoinHorizontal(lipgloss.Top,
			creditCard.Render(creditContent),
			"  ",
			msgCard.Render(msgContent),
			"  ",
			delCard.Render(delContent),
		)

		// detail rows
		var details []string
		details = append(details, "")
		details = append(details, fmt.Sprintf("  %s %s  %s %s  %s %s",
			sMuted.Render("Delivered:"), sSuccess.Render(fmt.Sprintf("%.0f", delivered)),
			sMuted.Render("Failed:"), sError.Render(fmt.Sprintf("%.0f", failed)),
			sMuted.Render("Total:"), sBold.Render(fmt.Sprintf("%.0f", sent)),
		))
		details = append(details, "")

		return commandResultMsg{
			title:  "Account · " + repl.mode,
			output: cards + "\n" + strings.Join(details, "\n"),
		}
	}
}

// ── /keys ────────────────────────────────────────────────────────

func (repl REPLModel) executeKeysCommand(arguments string) tea.Cmd {
	subcommand, subArgs := splitCommandLine(arguments)

	if subcommand == "create" {
		return repl.executeKeysCreateCommand(subArgs)
	}

	return repl.executeKeysListCommand()
}

func (repl REPLModel) executeKeysCreateCommand(modeArg string) tea.Cmd {
	return func() tea.Msg {
		mode := strings.ToUpper(strings.TrimSpace(modeArg))
		if mode == "" {
			mode = "LIVE"
		}
		if mode != "LIVE" && mode != "TEST" {
			return commandErrorMsg{errorText: "Usage: keys create <LIVE|TEST>"}
		}

		generatedKey, createError := repl.apiClient.CreateAPIKey(mode, replKeyName())
		if createError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed to create key: %s", createError.Error())}
		}

		card := sCardHighlight.Width(60)
		content := sSuccess.Render("✔ API Key Created") + "\n\n" +
			sMuted.Render("  Key:  ") + sBold.Render(generatedKey.PlainTextKey) + "\n" +
			sMuted.Render("  Mode: ") + renderModeBadge(mode) + "\n\n" +
			sWarn.Render("  ⚠ Save this key — you won't see it again!")

		return commandResultMsg{output: "\n" + card.Render(content) + "\n", raw: true}
	}
}

func (repl REPLModel) executeKeysListCommand() tea.Cmd {
	return func() tea.Msg {
		apiKeys, listError := repl.apiClient.ListAPIKeys()
		if listError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", listError.Error())}
		}

		if len(apiKeys) == 0 {
			return commandResultMsg{
				title:  "API Keys",
				output: sDim.Render("  No API keys yet. Run ") + sBlue.Render("keys create LIVE") + sDim.Render(" to mint one."),
			}
		}

		rows := make([][]string, 0, len(apiKeys))
		for _, keyInfo := range apiKeys {
			rows = append(rows, []string{
				sBrand.Render("• ") + sBold.Render(fmt.Sprintf("%s····%s", keyInfo.Prefix, keyInfo.LastFour)),
				apiKeyModeBadge(keyInfo.Mode),
				sDim.Render(formatTimeAgo(keyInfo.CreatedAt)),
			})
		}
		return commandResultMsg{
			title:  fmt.Sprintf("API Keys · %d", len(apiKeys)),
			output: renderREPLTable(len(apiKeys), []string{"KEY", "MODE", "CREATED"}, rows),
		}
	}
}

// ── /logs ────────────────────────────────────────────────────────

func (repl REPLModel) executeLogsCommand() tea.Cmd {
	return func() tea.Msg {
		mode := strings.ToLower(repl.mode)

		messages, messagesError := repl.apiClient.GetMessages(mode, 0, 10)
		if messagesError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed to fetch logs: %s", messagesError.Error())}
		}

		if messages == nil {
			return commandResultMsg{title: "Recent Messages", output: sDim.Render("  No recent messages")}
		}

		dataSlice, dataExists := messages["messages"]
		if !dataExists {
			return commandResultMsg{title: "Recent Messages", output: sDim.Render("  No recent messages")}
		}

		messageList, isList := dataSlice.([]any)
		if !isList || len(messageList) == 0 {
			return commandResultMsg{title: "Recent Messages", output: sDim.Render("  No recent messages")}
		}

		rows := make([][]string, 0, len(messageList))
		for _, item := range messageList {
			messageMap, isMap := item.(map[string]any)
			if !isMap {
				continue
			}
			status := extractString(messageMap, "status")
			receiver := strings.TrimPrefix(extractString(messageMap, "receiver"), "whatsapp:")
			templateName := extractString(messageMap, "templateName")
			createdAt := extractString(messageMap, "createdAt")

			if templateName == "" {
				templateName = "—"
			}

			rows = append(rows, []string{
				messageStatusBadge(status),
				sBrand.Render("• ") + sBold.Render(truncateStr(receiver, 16)),
				sMuted.Render(truncateStr(templateName, 22)),
				sDim.Render(formatTimeAgo(createdAt)),
			})
		}

		return commandResultMsg{
			title:  fmt.Sprintf("Recent Messages · %d", len(rows)),
			output: renderREPLTable(len(rows), []string{"STATUS", "PHONE", "TEMPLATE", "TIME"}, rows),
		}
	}
}

// messageStatusBadge maps a Message.status (whatsapp:* and internal
// enum values) to the shared pill palette. Defined here because logs is
// the only consumer; if a second caller appears, promote to repl_badges.go.
func messageStatusBadge(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DELIVERED":
		return sPillSuccess.Render("✓ DELIVERED")
	case "READ":
		return sPillSuccess.Render("✓ READ")
	case "SENT", "SENT_TO_PROVIDER":
		return sPillInfo.Render("→ SENT")
	case "PENDING", "QUEUED", "PROCESSING":
		return sPillWarn.Render("⧗ PENDING")
	case "FAILED":
		return sPillDanger.Render("✘ FAILED")
	case "":
		return sPillNeutral.Render("◌ UNKNOWN")
	default:
		return sPillNeutral.Render("◌ " + strings.ToUpper(strings.TrimSpace(status)))
	}
}

// ── /whoami ──────────────────────────────────────────────────────

func (repl REPLModel) renderWhoami() string {
	card := sCard.Width(45)

	content := sBold.Render("Current Session") + "\n\n" +
		sMuted.Render("  Profile:  ") + sValue.Render(repl.profileName) + "\n" +
		sMuted.Render("  Mode:     ") + renderModeBadge(repl.mode) + "\n" +
		sMuted.Render("  Version:  ") + sValue.Render(version.Version) + "\n" +
		sMuted.Render("  API:      ") + sDim.Render(config.DefaultAPIURL)

	return "\n" + card.Render(content) + "\n"
}

// ── /config ──────────────────────────────────────────────────────

func (repl REPLModel) renderConfig() string {
	configuration, loadError := config.Load()
	if loadError != nil {
		return sError.Render(fmt.Sprintf("  ✘ Failed to load config: %s", loadError.Error()))
	}

	card := sCard.Width(55)

	var profileLines []string
	for profileName, profile := range configuration.Profiles {
		marker := sDim.Render("  ○ ")
		if profileName == configuration.CurrentProfile {
			marker = sSuccess.Render("  ● ")
		}
		profileLines = append(profileLines, marker+sBold.Render(profileName)+
			sDim.Render(" · ")+sMuted.Render(profile.Mode)+
			sDim.Render(" · ")+sDim.Render(profile.APIURL))
	}

	content := sBold.Render("Configuration") + "\n\n" +
		sMuted.Render("  File: ") + sDim.Render(config.ConfigPath()) + "\n\n" +
		sMuted.Render("  Profiles:") + "\n" +
		strings.Join(profileLines, "\n")

	return "\n" + card.Render(content) + "\n"
}

// ── /campaigns ───────────────────────────────────────────────────

func (repl REPLModel) executeCampaignsCommand(arguments string) tea.Cmd {
	// `campaigns` and `campaigns list` are both aliases for the listing
	// view; anything else gets treated as a UUID and routed to the detail
	// view. Without this `list` branch the verb was being concatenated
	// onto the path (/v1/campaigns/list → 500 "No static resource").
	args := strings.TrimSpace(arguments)
	if args == "" || strings.EqualFold(args, "list") {
		return repl.executeCampaignsListCommand()
	}
	return repl.executeCampaignDetailCommand(args)
}

func (repl REPLModel) executeCampaignsListCommand() tea.Cmd {
	return func() tea.Msg {
		campaigns, listError := repl.apiClient.ListCampaigns()
		if listError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", listError.Error())}
		}

		if len(campaigns) == 0 {
			return commandResultMsg{
				title:  "Campaigns",
				output: sDim.Render("  No campaigns yet. Run ") + sBlue.Render("estimate <template> <count>") + sDim.Render(" to size your first one."),
			}
		}

		rows := make([][]string, 0, len(campaigns))
		for _, campaign := range campaigns {
			failedStyle := sDim
			if campaign.FailedCount() > 0 {
				failedStyle = sError
			}

			rows = append(rows, []string{
				sBrand.Render("• ") + sBold.Render(truncateStr(campaign.Name, 24)),
				campaignStatusBadge(campaign.Status),
				sMuted.Render(fmt.Sprintf("%d", campaign.TotalMessages)),
				failedStyle.Render(fmt.Sprintf("%d", campaign.FailedCount())),
				sValue.Render(fmt.Sprintf("R$ %.2f", campaign.TotalCost)),
			})
		}

		browsableRows := make([]browsableRow, 0, len(campaigns))
		for index := range campaigns {
			browsableRows = append(browsableRows, browsableRow{
				id:    campaigns[index].ID,
				cells: rows[index],
			})
		}
		return commandResultMsg{
			title:   fmt.Sprintf("Campaigns · %d", len(campaigns)),
			command: "campaigns",
			browsable: &browsableResult{
				title:      fmt.Sprintf("Campaigns · %d", len(campaigns)),
				command:    "campaigns",
				columns:    []string{"NAME", "STATUS", "TOTAL", "FAILED", "COST"},
				rows:       browsableRows,
				detailFunc: repl.campaignDetailFetcher(),
			},
		}
	}
}

// campaignDetailFetcher returns a closure that performs a GetCampaign on
// demand. We can't snapshot the list response like templates because the
// list payload is intentionally trimmed (CampaignListItem ≠
// CampaignDetailResponse) — drilling in needs the full record.
func (repl REPLModel) campaignDetailFetcher() func(string) (string, error) {
	return func(id string) (string, error) {
		campaign, err := repl.apiClient.GetCampaign(id)
		if err != nil {
			return "", err
		}
		return renderCampaignDetailBody(campaign), nil
	}
}

// renderCampaignDetailBody mirrors executeCampaignDetailCommand's body
// composer but without the surrounding panel — the drill area paints its
// own panel around the returned string.
func renderCampaignDetailBody(campaign *api.CampaignResponse) string {
	progress := 0
	if campaign.TotalMessages > 0 {
		progress = (campaign.SentCount * 100) / campaign.TotalMessages
	}
	bar := renderProgressBar(progress, 36)
	return sMuted.Render("ID        ") + sDim.Render(campaign.ID) + "\n" +
		sMuted.Render("Name      ") + sBold.Render(campaign.Name) + "\n" +
		sMuted.Render("Status    ") + campaignStatusBadge(campaign.Status) + "\n" +
		sMuted.Render("Progress  ") + sValue.Render(fmt.Sprintf("%d / %d", campaign.SentCount, campaign.TotalMessages)) +
		sDim.Render("  ") + bar + sDim.Render(fmt.Sprintf("  %d%%", progress)) + "\n" +
		sMuted.Render("Failed    ") + sError.Render(fmt.Sprintf("%d", campaign.FailedCount())) + "\n" +
		sMuted.Render("Cost      ") + sValue.Render(fmt.Sprintf("R$ %.2f", campaign.TotalCost))
}

func (repl REPLModel) executeCampaignDetailCommand(campaignID string) tea.Cmd {
	return func() tea.Msg {
		campaign, getError := repl.apiClient.GetCampaign(strings.TrimSpace(campaignID))
		if getError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", getError.Error())}
		}

		progress := 0
		if campaign.TotalMessages > 0 {
			progress = (campaign.SentCount * 100) / campaign.TotalMessages
		}

		progressBar := renderProgressBar(progress, 36)

		body := sMuted.Render("ID        ") + sDim.Render(campaign.ID) + "\n" +
			sMuted.Render("Status    ") + campaignStatusBadge(campaign.Status) + "\n" +
			sMuted.Render("Progress  ") + sValue.Render(fmt.Sprintf("%d / %d", campaign.SentCount, campaign.TotalMessages)) +
			sDim.Render("  ") + progressBar + sDim.Render(fmt.Sprintf("  %d%%", progress)) + "\n" +
			sMuted.Render("Failed    ") + sError.Render(fmt.Sprintf("%d", campaign.FailedCount())) + "\n" +
			sMuted.Render("Cost      ") + sValue.Render(fmt.Sprintf("R$ %.2f", campaign.TotalCost))

		return commandResultMsg{title: "Campaign · " + campaign.Name, output: body}
	}
}

// renderProgressBar draws a compact unicode progress bar — used in the
// campaign detail view, and reusable anywhere we want a quick at-a-glance
// completion cue. width is the *number of cells*, not characters; one cell
// = one ▰ or ▱.
func renderProgressBar(percent, width int) string {
	if width < 4 {
		width = 4
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := (percent * width) / 100
	if filled > width {
		filled = width
	}
	return sBrand.Render(strings.Repeat("▰", filled)) +
		sDim.Render(strings.Repeat("▱", width-filled))
}

// ── /contacts ────────────────────────────────────────────────────

func (repl REPLModel) executeContactsCommand(arguments string) tea.Cmd {
	subcommand, subArgs := splitCommandLine(arguments)

	switch subcommand {
	case "search":
		return repl.executeContactsSearchCommand(subArgs)
	case "stats":
		return repl.executeContactsStatsCommand()
	default:
		return repl.executeContactsListCommand(arguments)
	}
}

func (repl REPLModel) executeContactsListCommand(query string) tea.Cmd {
	return func() tea.Msg {
		response, listError := repl.apiClient.ListContacts(query, 0, 20)
		if listError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", listError.Error())}
		}

		if len(response.Contacts) == 0 {
			return commandResultMsg{output: sDim.Render("  No contacts found")}
		}

		rows := make([][]string, 0, len(response.Contacts))
		for _, contact := range response.Contacts {
			email := contact.Email
			if email == "" {
				email = sDim.Render("—")
			} else {
				email = sDim.Render(truncateStr(email, 25))
			}
			rows = append(rows, []string{
				sBrand.Render("• ") + sBold.Render(truncateStr(contact.Name, 22)),
				sMuted.Render(contact.Phone),
				email,
				sDim.Render(formatTimeAgo(contact.CreatedAt)),
			})
		}

		table := renderREPLTable(int(response.Total),
			[]string{"NAME", "PHONE", "EMAIL", "CREATED"}, rows)

		if response.TotalPages > 1 {
			table += "\n" + sDim.Render(fmt.Sprintf("  page %d of %d", response.Page+1, response.TotalPages))
		}
		return commandResultMsg{
			title:  fmt.Sprintf("Contacts · %d", response.Total),
			output: table,
		}
	}
}

func (repl REPLModel) executeContactsSearchCommand(query string) tea.Cmd {
	if strings.TrimSpace(query) == "" {
		return func() tea.Msg {
			return commandErrorMsg{errorText: "Usage: contacts search <name or phone>"}
		}
	}
	return repl.executeContactsListCommand(query)
}

func (repl REPLModel) executeContactsStatsCommand() tea.Cmd {
	return func() tea.Msg {
		stats, statsError := repl.apiClient.GetContactStats()
		if statsError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", statsError.Error())}
		}

		total := stats["total"]
		active := stats["active"]
		optedOut := stats["optedOut"]

		body := sMuted.Render("Total      ") + sValue.Render(fmt.Sprintf("%d", total)) + "\n" +
			sMuted.Render("Active     ") + sSuccess.Render(fmt.Sprintf("%d", active)) + "\n" +
			sMuted.Render("Opted-out  ") + sWarn.Render(fmt.Sprintf("%d", optedOut))

		return commandResultMsg{title: "Contact Statistics", output: body}
	}
}

// ── /numbers ─────────────────────────────────────────────────────

func (repl REPLModel) executeNumbersCommand() tea.Cmd {
	return func() tea.Msg {
		numbers, listError := repl.apiClient.ListNumbers()
		if listError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", listError.Error())}
		}

		if len(numbers) == 0 {
			return commandResultMsg{
				title:  "Phone Numbers",
				output: sDim.Render("  No phone numbers registered. Connect one at ararahq.com → Numbers."),
			}
		}

		rows := make([][]string, 0, len(numbers))
		for _, number := range numbers {
			name := number.Name
			if name == "" {
				name = sDim.Render("unnamed")
			} else {
				name = sBold.Render(name)
			}
			rows = append(rows, []string{
				sSuccess.Render("●"),
				sBrand.Render("• ") + sBold.Render(number.PhoneNumber),
				name,
			})
		}

		return commandResultMsg{
			title:  fmt.Sprintf("Phone Numbers · %d", len(numbers)),
			output: renderREPLTable(len(numbers), []string{"", "PHONE", "NAME"}, rows),
		}
	}
}

// ── /estimate ────────────────────────────────────────────────────

func (repl REPLModel) executeEstimateCommand(arguments string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.Fields(arguments)
		if len(parts) < 2 {
			return commandErrorMsg{errorText: "Usage: estimate <template-name> <contact-count>\n  Example: estimate welcome_msg 500"}
		}

		templateName := parts[0]
		countStr := parts[1]
		count := 0
		if _, scanErr := fmt.Sscanf(countStr, "%d", &count); scanErr != nil || count <= 0 {
			return commandErrorMsg{errorText: "Contact count must be a positive number"}
		}

		estimate, estimateError := repl.apiClient.EstimateCampaign(templateName, count)
		if estimateError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", estimateError.Error())}
		}

		body := sMuted.Render("Template     ") + sValue.Render(templateName) + "\n" +
			sMuted.Render("Category     ") + formatCategoryCell(estimate.TemplateCategory) + "\n" +
			sMuted.Render("Recipients   ") + sBold.Render(fmt.Sprintf("%d", estimate.RecipientCount)) + "\n" +
			sDim.Render(strings.Repeat("─", 36)) + "\n" +
			sMuted.Render("Template     ") + sDim.Render(fmt.Sprintf("R$ %.4f /msg", estimate.TemplateCost)) + "\n" +
			sMuted.Render("Arara fee    ") + sDim.Render(fmt.Sprintf("R$ %.4f /msg", estimate.AraraFee)) + "\n" +
			sMuted.Render("Unit price   ") + sValue.Render(fmt.Sprintf("R$ %.4f /msg", estimate.UnitPrice)) + "\n" +
			sDim.Render(strings.Repeat("─", 36)) + "\n" +
			sBold.Render("Total cost   ") +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cGreen)).Render(fmt.Sprintf("R$ %.2f", estimate.TotalCost))

		return commandResultMsg{title: "Cost Estimate", output: body}
	}
}

// ── /balance ─────────────────────────────────────────────────────

func (repl REPLModel) executeBalanceCommand() tea.Cmd {
	return func() tea.Msg {
		mode := strings.ToLower(repl.mode)
		walletBalance, walletError := repl.apiClient.GetWalletBalance(mode)
		if walletError != nil {
			return commandErrorMsg{errorText: fmt.Sprintf("Failed: %s", walletError.Error())}
		}

		balance := extractFloat(walletBalance, "balance")

		card := sCardHighlight.Width(35)
		content := sMuted.Render("Wallet Balance") + "\n\n" +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cGreen)).Render(fmt.Sprintf("  R$ %.2f", balance))

		return commandResultMsg{output: "\n" + card.Render(content) + "\n"}
	}
}

// ── /listen hint ─────────────────────────────────────────────────

func renderListenHint() string {
	card := sCard.Width(55)

	content := sBold.Render("Webhook Listener") + "\n\n" +
		sMuted.Render("  The listener runs in full-screen mode.") + "\n" +
		sMuted.Render("  Exit the REPL and run:") + "\n\n" +
		sValue.Render("  arara listen") + "\n\n" +
		sMuted.Render("  Options:") + "\n" +
		sDim.Render("    --forward-to http://localhost:3000/webhook") + "\n" +
		sDim.Render("    --events message.sent,message.delivered")

	return "\n" + card.Render(content) + "\n"
}

// ── Not logged in ────────────────────────────────────────────────

func renderNotLoggedIn() string {
	card := sCard.BorderForeground(lipgloss.Color(cRed)).Width(50)

	content := sError.Render("✘ Not authenticated") + "\n\n" +
		sMuted.Render("  Run: ") + sValue.Render("arara login --key <your-key>")

	return "\n" + card.Render(content) + "\n"
}

// ── Docs ─────────────────────────────────────────────────────────

func openDocsInBrowser() string {
	var openCommand string

	switch runtime.GOOS {
	case replPlatformMac:
		openCommand = replOpenCmdMac
	case replPlatformLinux:
		openCommand = replOpenCmdLinux
	default:
		return sDim.Render(fmt.Sprintf("  Open manually: %s", replDocsURL))
	}

	if startError := exec.Command(openCommand, replDocsURL).Start(); startError != nil {
		return sDim.Render(fmt.Sprintf("  Failed to open browser. Visit: %s", replDocsURL))
	}

	return sSuccess.Render(fmt.Sprintf("  ✔ Opening %s", replDocsURL))
}

// ── Helpers ──────────────────────────────────────────────────────

func truncateStr(input string, maxLength int) string {
	if len(input) <= maxLength {
		return input
	}

	if maxLength <= 3 {
		return input[:maxLength]
	}

	return input[:maxLength-3] + "..."
}

// ── Entry Point ──────────────────────────────────────────────────

func RunREPL() error {
	replModel := NewREPLModel()
	program := tea.NewProgram(replModel, tea.WithAltScreen())

	_, runError := program.Run()
	if runError != nil {
		return fmt.Errorf("failed to run REPL: %w", runError)
	}

	return nil
}
