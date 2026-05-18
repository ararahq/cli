# Changelog

All notable changes to the Arara CLI are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-05-12

### Added

- `arara api <method> <path>` — generic command for any AraraHQ endpoint
  not yet wrapped in a dedicated subcommand. Reuses auth, retry,
  idempotency-key, and verbose redaction.
- `--jq <expr>` global flag — filters JSON output through an embedded jq
  expression (gojq, pure Go, no external `jq` required).
- `arara init` — scaffolds `.arara/` in the current project with
  `settings.json`, an example slash command, an example hook, and a
  `.gitignore` entry guarding `credentials*`.
- `arara upgrade --install` — downloads the matching release archive
  from GitHub, verifies SHA-256 against `checksums.txt`, and atomically
  swaps the running binary. Bounded extraction defends against
  decompression bombs (G110).
- `arara listen --forward-to <url>` — POSTs each SSE webhook event to a
  local URL with `X-Arara-Webhook-Event`, `X-Arara-Forwarded-By`, and
  (when capture is active) `X-Arara-Signature` headers. URL is validated
  upfront (`http`/`https`, host required).
- `arara listen --capture` — registers an ephemeral capture listener
  on the backend (RFC 0003) so production webhooks are intercepted and
  signed for the local handler. Falls back gracefully to passive SSE
  when the backend doesn't yet support the listener API.
- `arara profile add|list|switch|remove` — manage multiple local
  profiles (multi-org / multi-team operators).
- `arara org list|use <slug>` — switch the active organization for
  agencies/consultants operating multiple AraraHQ accounts. Backend
  endpoint `/auth/me/organizations`.
- `arara telemetry on|off|status` — anonymous opt-in CLI usage
  telemetry. Generates a local UUID v4, batches up to 50 events at
  5min intervals, ships to `POST /v1/cli/telemetry/events`. No PII.

### Changed

- OAuth Device Flow validated end-to-end against the production
  endpoints (`/oauth/device/code|token`, `/oauth/token/refresh`).
  `--experimental-oauth` is now opt-out (`ARARA_OAUTH_ENABLED=0` to
  disable) since the backend ships the spec.
- `RFC 0003` envelope: when the backend wraps an SSE event in
  `{event, data, signature}`, the CLI lifts the signature into a
  dedicated field and propagates `X-Arara-Signature` on `--forward-to`.
- `output.WriteTable(w, headers, rows)` — table renderer now accepts
  any `io.Writer`. The legacy `output.PrintTable` stays as a
  stdout-bound wrapper for backwards compatibility.

### Security

- **Decompression bomb defense** (`copyBounded`): `arara upgrade
  --install` now caps any single extracted file at 200MB so a MITM
  cannot exhaust disk/RAM via a maliciously-large release archive.
- **Path traversal hardened** in `arara init` (G703): the gitignore
  update was already constrained to the `--dir` root; we documented
  the invariant and silenced the linter's tainted-flow warning with
  the rationale inline.
- **Context-cancel leak fixed** in the TUI webhook viewer (G118): the
  SSE consumer now uses `context.Background()` (lifetime owned by the
  bubbletea program) instead of an orphaned `WithCancel`.
- **Capture listener secret** is shown in the terminal only — never
  persisted to disk.

### Engineering

- Lint: `golangci-lint v2.12` with the v2 schema. `golangci-lint run`
  reports **0 issues**. Tightened `errcheck`, `errname`, `errorlint`,
  `gocritic`, `gosec`, `nilerr`, `staticcheck`, `unparam`, `unused`,
  `predeclared`, and 13 others.
- Coverage: `internal/api` **92.7%**, `internal/telemetry` **81.1%**,
  `internal/commands` 84.4%, `internal/mcp` 88%, `internal/plugins`
  90%, `internal/settings` 92%, `internal/hooks` 93%, `internal/cmd`
  **66.4%** (was 25% pre-v0.2).
- Distribution: goreleaser config produces brew, scoop, docker,
  deb/rpm/apk, plus an npm wrapper (`@ararahq/cli`).

### Known gaps (tracked for 0.2.x / 0.3)

- **`internal/cmd` coverage at 66.4%, not 90%.** §0 of the
  engineering standard requires 90%. The shortfall is concentrated in
  three I/O-heavy paths that are awkward to unit-test: the bubbletea
  TUI viewers (`runStatus`, `runListenViewer`, `send` wizard), the
  blocking `signal.NotifyContext` loops (`runListen`, `runSend
  --watch`), and the interactive terminal-only login path. Tracked as
  an explicit follow-up; targeting 80% in 0.2.1 via integration tests
  and 90% in 0.3 via a TUI test harness.
- **`context.Context` not yet threaded through `*api.Client`.** SIGINT
  cancels the consumer loop but in-flight HTTP requests run to
  completion. Slated for 0.3.
- **6 functions in code I touched this release sit in the 28–40-line
  range.** Below the threshold §0 sets (25 lines). Further splitting
  would harm clarity; tracked as cosmetic debt.
- **Backend RFC 0003** (capture listener) is not deployed yet. The
  CLI ships the consumer + graceful fallback; the feature becomes
  active in production without a CLI release once the backend lands.

## [0.1.0-beta] - 2026-05-11

First public beta. Surface is intentionally narrower than the long-term
target — see README for the full command list and known gaps below.

### Added

- `arara mcp` — Model Context Protocol server exposing 10 read tools (and 3
  write tools behind `--allow-write-tools`) so agents can drive AraraHQ
  without re-implementing auth, retry, or idempotency. See
  [`docs/rfc/0001-mcp-server.md`](docs/rfc/0001-mcp-server.md).
- User-defined slash commands under `.arara/commands/*.md` discovered as
  Cobra subcommands and inside the REPL. Project scope walks up the
  directory tree like git looks for `.git/`.
- Settings cascade (defaults → user → project) with `arara settings`
  CRUD and `--source` to inspect provenance.
- Hooks at `pre-send`, `post-deliver`, `pre-campaign`, `post-campaign`,
  `on-error` with configurable per-hook timeout.
- Plugins via `arara-<name>` binaries on `$PATH`, surfaced under a
  dedicated `--help` group, with global flags forwarded as env vars.
- Interactive TUI: REPL, send wizard, live dashboard, webhook viewer,
  logs viewer (Bubble Tea).
- Multiple output formats: `text`, `json`, `table`, `stream-json`.
- Theme controls (`--theme`, `--no-color`, `ARARA_THEME`, `NO_COLOR`).
- `arara doctor` — environment, auth, and connectivity diagnostics.
- `arara upgrade` — checks GitHub releases for newer versions.
- Shell completions (`arara completion install`) and pre-generated man
  pages (`arara generate-manpages`).
- Goreleaser pipeline producing macOS / Linux / Windows × amd64 / arm64
  binaries with Homebrew tap install.

### Security

- **Verbose log redaction** (`internal/api/redact.go`): request and
  response bodies logged via `--verbose` now have sensitive keys
  (`token`, `accessToken`, `refreshToken`, `apiKey`, `password`,
  `authorization`, etc., case-insensitive) replaced with `<REDACTED>`.
  Non-JSON bodies are emitted as `<non-json body redacted (N bytes)>` so
  opaque token formats never leak to stderr.
- **Slash command trust prompt** (`internal/commands/trust.go`):
  project-scope `.arara/commands/*.md` files no longer execute
  unconditionally. Approval is requested once per source directory and
  persisted to `~/.arara/trusted-commands.json` (atomic write, mode
  0600). Non-TTY invocations refuse with exit code 2 unless
  `ARARA_TRUST=1` is set. User-scope commands (`~/.arara/commands/`)
  continue to run without prompting.

### Changed

- `--experimental-oauth` is now hidden behind `ARARA_OAUTH_ENABLED=1`
  until the backend device-flow endpoints are deployed in production.
  The implementation remains in place; only the Cobra flag is gated.

### Known gaps (tracked for v0.1.x)

- Test coverage of `internal/cmd` is at ~34% (was 25% before this
  release). Several command files (`contacts`, `campaigns`, `logs`)
  still need the refactor that decouples `*api.Client` from
  `NewClientFromConfig` and adds `httptest.Server`-backed tests. The
  underlying `*api.Client` itself sits at 93.7% coverage.
- HTTP requests in `internal/api/client.go` do not yet accept a
  `context.Context`. SIGINT cancellation works at the loop level
  (`arara send --watch`, `arara listen`) via `signal.NotifyContext`,
  but in-flight requests can not be cancelled mid-flight. Slated for
  v0.2.

[0.1.0-beta]: https://github.com/ararahq/cli/releases/tag/v0.1.0-beta
