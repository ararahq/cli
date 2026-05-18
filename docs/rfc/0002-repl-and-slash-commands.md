# RFC 0002: REPL Stabilization + Slash Commands

- **Status**: Implemented (both Phase 1 stabilization and Phase 2 slash commands)
- **Author(s)**: TBD
- **Created**: 2026-05-10
- **Updated**: 2026-05-10
- **Related**: `internal/tui/repl.go`, [RFC 0001](./0001-mcp-server.md)
- **Phase 1 Implementation**: `internal/tui/repl_dispatch.go`, refactor of `internal/tui/repl.go`, theme wiring in `internal/output/colors.go`
- **Phase 2 Implementation**: `internal/commands/`, `internal/cmd/commands_register.go`, `internal/cmd/commands_cmd.go`, REPL dispatch via `executeSlashCommand` in `internal/tui/repl.go`

## Phase 1 Shipped — Notes

What landed:

- **`ParseInput`** (`repl_dispatch.go`) extracts the input → typed `Action` mapping into a pure function with 100% test coverage. The Bubble Tea `Update` handler now does a type switch on the returned `Action`, which is easier to read and easier to extend (e.g., Phase 2 will add `ActionRunSlashCommand` to this enum).
- **`splitCommandLine`** replaces the duplicated `parseCommand` helper that was inlined in both `executeKeysCommand` and `executeCampaignsCommand`.
- **Dead code removed**: `sTableHeader` (unused style var) and `extractStringFromMap` (unused helper).
- **All 14 deprecated `lipgloss.Style.Copy()` calls** replaced with direct chained calls (lipgloss v1.x style methods already return by value).
- **Theme wiring**: `output.ApplyTheme(ThemeMono)` now also calls `lipgloss.SetColorProfile(termenv.Ascii)`, which means **the whole REPL respects `--theme=mono`/`ARARA_THEME=mono` automatically** without rewriting every render call site. Universal effect at the cost of one line — the right trade-off for a 1,300-line view file.
- **`internal/tui/`** staticcheck: clean (was 16 warnings).

What was punted from the original spec:

- **Coverage ≥ 80% on `internal/tui/`** — we hit ~2% globally because the view/executor code (`executeSendCommand`, `executeTemplatesCommand`, the dashboard, etc.) needs `teatest`-style integration tests to be meaningfully covered. The extracted pure logic *is* at 100%, but the view code remains untested. Achieving 80% on the package requires a dedicated TUI testing sprint with `charmbracelet/x/exp/teatest`. Tracked as a follow-up; not blocking Phase 2.
- **Replacing all REPL-local styles** (`sBrand`, `sSuccess`, ...) with `output.*Style` references — solved more cheaply via the global `SetColorProfile` hack above. Worth revisiting if a future theme adds a colour palette change (light/dark variants).

## Phase 2 Shipped — Notes

What landed:

- **`internal/commands` package** — `Loader.Discover()` cascades user (`~/.arara/commands/*.md`) → project (`./.arara/commands/*.md` walking up the tree). Frontmatter parsed via the existing `go.yaml.in/yaml/v3` dependency (no new deps). 92% coverage.
- **Argument interpolation** — `$1..$9`, `$@`, `$$` exactly as specified, with shell-quoting for safety. Bare words (alphanumerics + `_-.+/:@`) bypass quoting so things like phone numbers stay readable.
- **Cobra registration** — every discovered command becomes a top-level subcommand with `DisableFlagParsing` so args pass through verbatim. Built-ins always win on name collision (we warn and skip the slash command). Plugin commands also win.
- **`arara commands list/show/which`** — introspection: tabular list, full body display, source path lookup.
- **REPL dispatch** — slash strings (`/foo`) hit the `ActionUnknown` branch, which now consults the loader before printing the unknown-command error. The execution is async via a `tea.Cmd` (same pattern as built-in REPL commands), with stdout/stderr captured and rendered into the REPL output area.
- **`confirm: true` frontmatter field** — prompts y/N on stderr before running. The simplest safety net before we build the project-trust system.

What was punted from the original spec:

- **Per-project trust prompt** (Q1) — not implemented; the safer-by-default behavior of "built-ins always win" reduces the risk to "user typed `arara <project-command>` without inspecting the project". The README documents the risk. A real trust file with content hashes (per RFC) tracked as a follow-up.
- **Slash commands exposed as MCP tools** (Q6) — explicitly deferred. Letting agents execute arbitrary local shell needs its own RFC.
- **Argument validation via JSON Schema** (Q3) — deferred. Positional args + shell are sufficient for v1.

## Summary

Two related changes, in order:

1. **Stabilize the existing REPL** (`internal/tui/repl.go`, ~1.5k LOC, 0% test coverage). Refactor it to use `internal/output` styles instead of its own duplicated palette, remove dead code, extract domain logic from view code, and ship integration tests so future changes can land safely.
2. **Add slash commands**: project-local + user-local `.md` files under `.arara/commands/` that get registered as REPL commands (`/promo`, `/diag`, `/tail`) and as Cobra subcommands (`arara promo`).

The slash commands feature is the visible deliverable; stabilizing the REPL is the prerequisite that makes it safe to ship.

## Motivation

**REPL today:**

- Started life as a quick TUI demo. Now it's the entrypoint when a user runs `arara` in an interactive terminal (`rootCmd.RunE`).
- Has its own colour palette and styles (`internal/tui/repl.go:30-110`) that duplicate `internal/output/colors.go`. Theme changes (Sprint 2) don't propagate.
- Has dead code (`sTableHeader`, `extractStringFromMap`) and uses lipgloss APIs that are deprecated as of v1.1.0 (`.Copy()`).
- Zero tests. Refactoring it is a leap of faith.

**Slash commands today:**

- Don't exist.
- Power users keep asking for "Claude Code style" custom commands so they can save reusable workflows (`/blast-promo`, `/spend-today`, `/template hello_world`).
- Without a story for this, the only way to add a workflow is shell scripts in the user's `bin/` — which loses access to the active session, output formatter, and theme.

These two issues compound: we can't safely add slash commands on top of an untested REPL. So this RFC sequences them.

## Guide-level explanation

### Phase 1: REPL stabilization (invisible to users)

No user-visible change. Internally:

- REPL stops carrying its own palette; it imports `internal/output` styles. `--theme=mono` and `ARARA_THEME` now affect the REPL.
- Domain logic (parse user input, dispatch to handler, format result) is split from the Bubble Tea model. Each piece is unit-testable.
- Integration tests use `teatest` (Bubble Tea's test harness, [`charmbracelet/x/exp/teatest`](https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest)) to drive scripted sessions.
- Patch coverage on `internal/tui/repl.go` reaches ≥80%.

### Phase 2: Slash commands

A user creates `.arara/commands/spend-today.md` in their project:

```markdown
---
description: Show today's send count and credit burn
argHint: ""
---

arara status --since today --output json | jq '{ sent: .messages.sent, cost: .billing.charged }'
```

In the REPL, they type:

```
❯ /spend-today
{
  "sent": 1247,
  "cost": 78.43
}
```

In any terminal, they can also run it as a regular subcommand:

```
$ arara spend-today
```

**Discovery order**:

1. Project: `./.arara/commands/*.md` (walking up the dir tree like `internal/settings`).
2. User: `~/.arara/commands/*.md`.
3. Built-in commands always win on collision.

**Argument interpolation**: `$1`, `$2`, ..., `$@` (Bash-style) are substituted from REPL args:

```
❯ /template hello_world Joao "Wed 14:00"
```
runs `.arara/commands/template.md` with `$1=hello_world`, `$2=Joao`, `$3=Wed 14:00`, `$@=hello_world Joao "Wed 14:00"`.

**Frontmatter fields** (all optional):

| Field | Default | Purpose |
|---|---|---|
| `description` | first body line | Shown in `/help` and `arara --help` |
| `argHint` | `""` | Help text after the command name |
| `confirm` | `false` | Prompt `Run? [y/N]` before executing |
| `cwd` | inherits | Override working directory |

The body is **executed via `sh -c`** (or `cmd.exe /C` on Windows). It is *not* sandboxed — same trust model as the user's shell rc files.

### Listing and inspecting

```
❯ /help
Built-in commands:
  /help, /quit, /clear, /history
  /tail, /spend, ...

User commands (~/.arara/commands):
  /diag             - Dump diagnostics for support
  /tail             - Tail webhooks with NDJSON

Project commands (./.arara/commands):
  /spend-today      - Show today's send count and credit burn
  /promo            - Run the seasonal promo campaign
```

```
$ arara commands list           # outside the REPL too
$ arara commands show diag      # show the file contents + resolved source
```

## Reference-level explanation

### Phase 1: REPL refactor

Affected files:

```
internal/tui/repl.go            (current; refactor target)
internal/tui/repl_model.go      (new; pure tea.Model)
internal/tui/repl_dispatch.go   (new; input parser + handler dispatch, no tea types)
internal/tui/repl_test.go       (new; teatest-based integration tests)
```

**Refactor steps:**

1. Replace local `sBrand`, `sSuccess`, ... with imports from `internal/output`.
2. Drop `.Copy()` calls (lipgloss methods already return new styles in v1.x).
3. Delete `sTableHeader` and `extractStringFromMap` (unused).
4. Extract input parsing into `parseREPLInput(line string) Action` returning a typed action (run subcommand, slash command, builtin, blank, exit).
5. Extract dispatch into `dispatchAction(ctx, deps, action) Result`. `deps` carries an `*api.Client`, settings, output theme. `Result` is a struct, not a tea.Msg.
6. Update the Bubble Tea model so `Update` calls `dispatchAction` and converts the `Result` into messages. The model becomes a thin shell.
7. Add unit tests for `parseREPLInput` (table-driven over input strings) and `dispatchAction` (mock deps, assert results).
8. Add integration tests with `teatest`: launch the REPL, send keystrokes, assert visible state.

**Acceptance criteria:**

- `internal/tui/` coverage ≥ 80%.
- `staticcheck` clean (currently 16 warnings in this file).
- No behavioural change for end users.

### Phase 2: Slash command loader

New package:

```
internal/commands/
├── loader.go       # discover + parse .md files
├── registry.go     # in-memory registry, Lookup(name)
├── execute.go      # arg interpolation + sh -c
└── *_test.go
```

```go
type Command struct {
    Name        string
    Description string
    ArgHint     string
    Confirm     bool
    Cwd         string
    Body        string  // raw shell command, post-interpolation
    Source      string  // "project" | "user"
    SourcePath  string  // absolute path to the .md file
}

type Loader struct {
    UserDir    string  // default ~/.arara/commands
    ProjectDir string  // default <cwd-walk>/.arara/commands
}

func (l *Loader) Discover() ([]Command, error)
```

Frontmatter parser: use [`adrg/frontmatter`](https://pkg.go.dev/github.com/adrg/frontmatter) (~50 LOC of standard YAML frontmatter support, MIT, well-tested).

### Wiring into the REPL

In `dispatchAction`:

- Strings starting with `/` become `slashCommand` actions.
- The dispatcher looks up the name in the registry; if found, runs interpolation and `exec.Command("sh", "-c", body)`.
- On Windows, use `cmd.exe /C` instead.
- `confirm: true` triggers a y/N prompt in the model layer.

### Wiring into Cobra

`internal/cmd/commands_register.go` (parallel to `plugins_register.go`):

- Called from `Execute()` after plugin registration.
- For each discovered command, register a Cobra subcommand whose `RunE` runs the shell body with `os.Args[2:]` as positional args.
- Built-in commands always win — collisions log a warning, the user command is skipped.
- `arara commands list / show / which` for inspection.

### Argument interpolation

```go
func Interpolate(body string, args []string) string {
    // $1, $2, ... → args[0], args[1], ...
    // $@ → joined args (shell-quoted)
    // $$ → literal $
    // Anything else passed through unchanged for the shell to handle.
}
```

Shell-quote `$@` substitutions to prevent re-tokenization. `$1`-style substitutions are also shell-quoted unless the user writes `${1!}` (force unquoted). This is important: an unquoted phone number like `+5511999999999` shouldn't break if the slash command body wraps it in quotes itself.

### Security considerations

Slash commands execute shell code with the user's full privileges. Mitigations:

- **No remote loading**: commands come only from `./.arara/commands/` and `~/.arara/commands/`. No HTTPS-fetched commands, no plugin-supplied commands (yet).
- **Visibility**: `arara commands list --source` shows where each one came from. `arara commands show <name>` prints the body.
- **Project safety**: when `arara` enters a directory with a project-level `.arara/commands/` for the first time, prompt the user once: `"This project defines slash commands. Trust them? [y/N]"`. Persist the trust marker as a hash in `~/.arara/trusted-projects.json`. Re-prompt if the directory's hash changes (similar to VS Code's workspace trust).
- **Confirm flag**: command authors mark destructive commands with `confirm: true`.

### Test plan

- **Loader**: `Discover` against fixture directories with valid + invalid frontmatter, conflicting names, missing dir, etc.
- **Interpolation**: table-driven over `(body, args) → expected_string`, including edge cases (trailing `$`, `$0`, mixed quoted/unquoted, special chars).
- **Execute**: run real `sh -c`/`cmd.exe /C` against fixture commands in `t.TempDir()`. Verify exit code propagation.
- **Trust prompt**: tested in a separate test suite that mocks the prompt input.
- **End-to-end**: `arara <slash-command-as-subcommand>` with a temp project containing a `.arara/commands/foo.md`.

## Drawbacks

1. **Shell injection by design**: slash command bodies are arbitrary shell. A malicious project-level command could run `rm -rf ~/`. We ship the trust prompt as the mitigation; users still need to read commands they install. We accept this as the cost of "scriptable workflows".
2. **REPL refactor risk**: 1.5k LOC, 0% coverage today. Changes can introduce subtle regressions. Phase 1 explicitly carves out time for this risk before adding any new features on top.
3. **Two ways to invoke a command**: as a slash command in the REPL and as a subcommand from the shell. Maintaining symmetry is annoying. Worth it: agents (RFC 0001) get every slash command for free as a subcommand.
4. **Frontmatter dependency**: `adrg/frontmatter` is small (~50 LOC) but is another dep. Acceptable; the alternative is hand-rolling YAML frontmatter parsing, which is bug-prone.
5. **Windows shell semantics differ from POSIX** (`sh -c` vs `cmd.exe /C`, quoting differences). We document this; non-portable command authors are expected to ship platform-specific bodies via two separate `.md` files (e.g., `foo.md` and `foo-windows.md`).

## Rationale and alternatives

### REPL refactor

| Option | Pros | Cons |
|---|---|---|
| **Refactor in place** *(recommended)* | Preserves git history; incremental tests catch regressions. | Requires discipline to not feature-creep. |
| **Rewrite from scratch** | Clean slate. | Loses keyboard shortcuts, scrollback, output formatting that already work. |
| **Replace with a third-party REPL framework** | Less code we own. | Tight coupling between Bubble Tea model and our domain — a generic REPL framework wouldn't know about templates/messages/etc. |

### Slash command file format

| Option | Pros | Cons |
|---|---|---|
| **`.md` with YAML frontmatter** *(recommended)* | Same convention as Claude Code custom commands; familiar; renders nicely on GitHub. | YAML quirks. |
| **`.toml` files** | Strict schema, no whitespace surprises. | Less natural for prose-heavy commands. |
| **`.sh` files with magic comments (`# arara: description = ...`)** | Lowest barrier. | Hard to add fields without re-grepping; clashes with shell linting. |
| **`.json` files** | Trivial to parse. | Authoring multi-line shell in JSON is painful. |

### Argument interpolation syntax

| Option | Pros | Cons |
|---|---|---|
| **`$1`, `$2`, `$@`** *(recommended)* | Bash-native; users already know it. | Conflicts with shell's own `$1` if the body inadvertently contains it. We document escaping (`$$1`). |
| **`{{ .arg1 }}` (Go templates)** | No shell collision. | Requires teaching template syntax; verbose. |
| **`%1%`, `%@%`** | No conflicts. | Unfamiliar; cmd.exe-isms. |

### Shell-out vs in-process

We *could* parse the slash command body and re-dispatch to Cobra in-process (no shell needed for `arara send ...` bodies). We've decided against that for v1 because:

- It would only handle `arara <subcommand>` lines; users want pipes, redirects, conditionals.
- The composability story (`| jq`, `| tee`, `&&`) is the whole point.
- Shelling out is what every other CLI scripting story does (`gh alias`, `kubectl alias`, shell aliases).

### Do nothing

Without slash commands, power users keep writing standalone scripts in `~/bin`. They lose access to active profiles, themes, and the REPL session context. The "platform" story (Sprint 3 onwards) is incomplete.

## Prior art

- **Claude Code custom commands**: project + user `.md` files with frontmatter, `$ARGUMENTS` interpolation, executed in-process via Claude. We borrow the file format and discovery model; we differ on execution (shell, not LLM).
- **`gh alias set`**: stores aliases in a config file. Single-line only; no multi-line bodies. Less flexible.
- **Git aliases**: `[alias]` block in `~/.gitconfig`. Same single-line limitation.
- **`kubectl plugins`** (closer to our existing PATH-plugin story; complementary, not replacement).
- **`fish` and `nushell` custom functions**: closest in spirit; we approximate via shell-out.

## Unresolved questions

- [ ] **Q1**: Trust prompt UX — single y/N on first encounter, or per-command on first use? Recommendation: per-directory, with re-prompt when files change.
- [ ] **Q2**: Should we sandbox commands by default (e.g., prepend `set -euo pipefail; ulimit -t 30` on POSIX)? Risk of breaking valid commands. Recommendation: no sandbox in v1; document best practice.
- [ ] **Q3**: Argument types — should frontmatter declare typed args (`args: [{name: phone, type: phone}]`) for validation before exec? Recommendation: defer; positional shell args are fine to start.
- [ ] **Q4**: Slash command discovery in subdirectories — walk up like settings, or only the cwd? Recommendation: walk up (matches settings).
- [ ] **Q5**: Should the REPL ship its own command for managing slash commands (`/cmd new`, `/cmd edit`)? Recommendation: defer; users edit `.md` files in their editor.
- [ ] **Q6**: Should commands be exposed to MCP agents (RFC 0001) as tools? E.g., `.arara/commands/promo.md` becomes a tool `arara_promo` callable by Claude. Tempting, but the security implication (agents executing local shell) needs its own RFC.
- [ ] **Q7**: How do we handle command conflicts with plugins (RFC-discoverable `arara-foo` vs slash command `foo`)? Recommendation: built-in > plugin > project slash > user slash, log warnings on collision.

## Future possibilities

- **Argument validation** via JSON Schema in frontmatter, applied before exec.
- **Streaming output** in slash commands — let the body write NDJSON to stdout and the REPL renders it as a live table.
- **Sharing commands**: `arara commands install <git-url>/path/to/commands` (depends on Q1 trust model).
- **Editor integration**: `arara commands edit <name>` opens the `.md` file in `$EDITOR`.
- **Slash commands as MCP tools** (gated by RFC TBD; see Q6).
- **REPL `--continue` / `--resume`**: persistent session history with named restore points (depends on REPL refactor).
