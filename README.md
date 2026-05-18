# Arara CLI

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Docs](https://img.shields.io/badge/Docs-docs.ararahq.com-orange)](https://docs.ararahq.com)

The official command-line interface for [AraraHQ](https://ararahq.com). Send WhatsApp messages, manage templates, run campaigns, and monitor everything — right from your terminal.

## Installation

### One-liner (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/ararahq/cli/main/install.sh | sh
```

Detects your OS + architecture, downloads the matching binary from GitHub Releases, verifies its SHA256 checksum, and installs to `/usr/local/bin` (falls back to `~/.local/bin` when not writable). Pin a version or change the install dir via env vars — see [`install.sh`](install.sh) for the full options.

### One-liner (Windows / PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/ararahq/cli/main/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\arara\bin` and adds it to your user PATH. No admin rights required.

### Homebrew (macOS / Linux)

```bash
brew install ararahq/tap/arara
```

### Go Install

```bash
go install github.com/ararahq/cli/cmd/arara@latest
```

### Binary Download

Download the latest release from [GitHub Releases](https://github.com/ararahq/cli/releases).

## Quick Start

```bash
# Authenticate
arara login

# Send a template message
arara send --to +5511999999999 --template hello_world --vars "João"

# Send a freeform message (requires active session)
arara send --to +5511999999999 --body "Hello from the CLI!"

# Preview the payload + cost without sending
arara send --to +5511999999999 --template hello_world --vars "João" --dry-run

# Block until the message reaches a terminal status
arara send --to +5511999999999 --template hello_world --watch

# Interactive wizard mode (no flags = TUI wizard)
arara send

# Diagnose your environment (version, auth, connectivity)
arara doctor
```

## Commands

| Command | Description |
|---------|-------------|
| `arara login` | Authenticate with your Arara account |
| `arara logout` | Clear stored credentials |
| `arara whoami` | Show current authenticated user |
| `arara send` | Send a WhatsApp message (template or freeform) |
| `arara templates` | List and manage WhatsApp templates |
| `arara campaigns` | Create and manage bulk messaging campaigns |
| `arara contacts` | Manage your contact list |
| `arara numbers` | Manage phone numbers |
| `arara keys` | Manage API keys (list, create, revoke) |
| `arara logs` | View message history and logs |
| `arara listen` | Real-time message listener (webhooks) |
| `arara status` | Live dashboard with auto-refresh (aliases: `dash`, `dashboard`) |
| `arara config` | Configure CLI settings (list, set, get) |
| `arara upgrade` | Update CLI to latest version |
| `arara docs` | Open documentation |
| `arara doctor` | Diagnose CLI environment, auth, and connectivity |
| `arara completion` | Generate or install shell completions |
| `arara sessions` | List and revoke CLI sessions (experimental) |
| `arara settings` | Manage non-credential settings (theme, hooks, plugins) |
| `arara mcp` | Run as a Model Context Protocol server for AI agents |
| `arara commands` | List/show/locate user-defined slash commands |

## Features

### Interactive TUI

The CLI includes a rich Terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea):

- Interactive message sending wizard (run `arara send` with no flags)
- Live dashboard with auto-refresh every 30s (`arara status`)
- Real-time message listener with live updates (`arara listen`)
- Formatted output with colors and tables

### Secure Credential Storage

Credentials are stored securely using your system's native keyring (macOS Keychain, Linux Secret Service, Windows Credential Manager) via [go-keyring](https://github.com/zalando/go-keyring).

### Multiple Output Formats

```bash
# Default: human-readable with colors
arara logs

# Pretty JSON
arara logs -o json

# NDJSON (one event per line) — pipe to jq, datadog, etc
arara listen -o stream-json | jq -r '.event'
arara logs --follow | tee log.ndjson

# Watch a single message via NDJSON (one line per state change)
arara send --to +5511999999999 --template hello_world --watch -o stream-json
```

### Theme & Color Control

```bash
# Disable colors (pipe-safe, no ANSI escapes)
arara doctor --theme mono
arara doctor --no-color

# Persist via env var
export ARARA_THEME=mono   # or auto (default)
export NO_COLOR=1         # standard, also forces mono
```

### Shell Completions

```bash
# Auto-detect from $SHELL and install to the conventional path
arara completion install

# Or explicitly target a shell
arara completion install zsh
arara completion install --shell bash

# Or pipe to your own location
arara completion zsh > ~/.zfunc/_arara
```

### Settings (Cascading)

Non-credential settings live in `settings.json` and cascade across three layers:

1. **defaults** (built-in)
2. **user** — `~/.arara/settings.json`
3. **project** — `./.arara/settings.json` (walks up the directory tree)

```bash
# See resolved settings + which layer each value came from
arara settings list --source

# Read a single value
arara settings get hooks.preSend

# Write at user scope (default) or project scope
arara settings set theme mono
arara settings set hooks.preSend "~/hooks/log-send.sh,~/hooks/datadog.sh" --project
```

Known paths: `theme`, `output`, `hookTimeoutSeconds`, `hooks.{preSend,postDeliver,preCampaign,postCampaign,onError}`, `plugins.{searchPaths,disabled}`, `experimental.oauth`.

### Hooks

Run shell scripts at lifecycle events. Configured via settings; receives event metadata via env + payload via stdin (NDJSON).

```bash
arara settings set hooks.preSend "~/hooks/log-send.sh"
```

```bash
# ~/hooks/log-send.sh
#!/usr/bin/env bash
echo "[$(date)] arara $ARARA_EVENT (profile=$ARARA_PROFILE)" >> ~/.arara/audit.log
cat   # consume payload from stdin (JSON)
exit 0   # non-zero on preSend / preCampaign aborts the operation
```

Available events: `pre-send`, `post-deliver`, `pre-campaign`, `post-campaign`, `on-error`. Per-hook timeout via `hookTimeoutSeconds` (default 30s). `pre-*` events abort on non-zero exit; `post-*` and `on-error` log and continue.

### Plugins

Drop a binary named `arara-<name>` anywhere in `$PATH` (or in `plugins.searchPaths` from settings) and it shows up in `arara --help` automatically — same pattern as `git`, `kubectl`, `gh`.

```bash
# ~/bin/arara-promo
#!/usr/bin/env bash
[ "$1" = "--plugin-description" ] && { echo "Run promo campaigns"; exit 0; }
echo "Running promo with profile=$ARARA_PROFILE..."
```

```bash
arara promo                # → runs ~/bin/arara-promo
arara settings set plugins.disabled "promo"   # disable
```

Plugins receive global flags via env: `ARARA_PROFILE`, `ARARA_OUTPUT`, `ARARA_MODE`, `ARARA_VERBOSE`. They're invoked with `DisableFlagParsing` so all args go through verbatim.

### User-Defined Slash Commands

Save reusable workflows as `.md` files under `.arara/commands/`. They show up as Cobra subcommands AND as slash commands inside the REPL.

**Example** — `./.arara/commands/spend-today.md`:

```markdown
---
description: Show today's send count and credit burn
argHint: ""
---
arara status --since today --output json | jq '{ sent: .messages.sent, cost: .billing.charged }'
```

Run it:

```bash
$ arara spend-today          # from any terminal
$ arara                      # then inside REPL:
❯ /spend-today
```

**Discovery cascade** (project wins on collision):

1. **Project**: `./.arara/commands/*.md` — walks up the directory tree like git looks for `.git/`
2. **User**: `~/.arara/commands/*.md`
3. **Built-in & plugin commands always win** over slash commands — a project can't override `arara send`

**Frontmatter fields** (all optional):

| Field | Default | Purpose |
|---|---|---|
| `description` | first non-comment body line | Shown in `arara commands list` and `--help` |
| `argHint` | `""` | Shown after the command name in help (e.g., `<phone>`) |
| `confirm` | `false` | Prompt `Run? [y/N]` before executing |
| `cwd` | inherits | Override working directory |

**Argument interpolation** (Bash-style):

```bash
arara my-cmd hello "world wide"
```
With body `echo $1 says $2`, becomes `echo hello says 'world wide'`. Values are auto-quoted to prevent splitting. Use `$@` for all args joined, `$$` for a literal `$`.

**Inspect what's installed:**

```bash
arara commands list                   # table view
arara commands list -o json           # machine-readable
arara commands show spend-today       # body + metadata
arara commands which spend-today      # source file path
```

**Security note:** slash command bodies are arbitrary shell. They run with your full user privileges. Treat a project's `.arara/commands/` like its `Makefile` — only run `arara <user-cmd>` if you trust the project. Project commands cannot override built-in commands, so `arara send` is always our `send`, not the project's. A persistent trust prompt is planned (see RFC 0002 Q1).

### MCP Server (for AI Agents)

`arara` doubles as a [Model Context Protocol](https://modelcontextprotocol.io) server. AI agents (Claude Code, Cursor, custom MCP clients) can call a curated subset of AraraHQ tools without re-implementing auth, retry, or idempotency — they reuse the CLI's authenticated session.

**Setup for Claude Code:**

```json
// ~/.claude/mcp_servers.json
{
  "arara": {
    "command": "arara",
    "args": ["mcp"],
    "env": { "ARARA_PROFILE": "production" }
  }
}
```

**Inspect available tools without serving:**

```bash
arara mcp --dry-run                    # 10 read-only tools
arara mcp --dry-run --allow-write-tools # 13 tools (adds send/create/import)
```

**Read tools (always exposed):**

| Tool | Description |
|---|---|
| `arara_list_templates` | List all WhatsApp templates |
| `arara_get_template_status` | Check template approval state |
| `arara_get_message_status` | Look up delivery status by message ID |
| `arara_list_contacts` | Paginated contact list with optional query |
| `arara_get_contact` | Fetch a single contact by phone |
| `arara_get_contact_stats` | Aggregate contact stats |
| `arara_get_metrics` | Dashboard metrics (volume, delivery rate) |
| `arara_get_wallet_balance` | Current credit balance |
| `arara_list_numbers` | Registered WhatsApp numbers |
| `arara_estimate_campaign` | Pre-compute campaign cost |

**Write tools (opt-in):**

Mutating tools are **disabled by default**. Two ways to enable:

```bash
# Per-invocation:
arara mcp --allow-write-tools

# Persisted in settings (user scope):
arara settings set mcp.allowWriteTools true
```

| Tool | Description |
|---|---|
| `arara_send_message` | Send a WhatsApp message (supports `dryRun: true` to preview cost) |
| `arara_create_campaign` | Launch a bulk campaign with idempotency key |
| `arara_import_contacts` | Bulk-import contacts, returns import ID |

Tool calls reuse the same retry + jitter + idempotency-key logic as the CLI. Agents can supply their own `idempotencyKey` argument for retry-safe semantics across MCP reconnects. Errors follow a consistent envelope:

```json
{
  "ok": false,
  "error": {
    "code": "ararahq-63016",
    "message": "Janela de 24h fechada. Envie um template primeiro.",
    "retryable": false,
    "retryAfterMs": 0
  },
  "statusCode": 400
}
```

See [`docs/rfc/0001-mcp-server.md`](docs/rfc/0001-mcp-server.md) for the design rationale.

### Experimental: OAuth Device Flow

> **Hidden by default.** The flag is only registered when `ARARA_OAUTH_ENABLED=1` is set, since the backend endpoints aren't deployed in production yet.

```bash
export ARARA_OAUTH_ENABLED=1
arara login --experimental-oauth
# → opens https://ararahq.com/cli/auth?code=BXTZ-9KQM
# → enter the user code, approve, CLI receives an access token
```

This is **experimental** and depends on backend endpoints that may not be deployed yet (`/oauth/device/code`, `/oauth/device/token`, `/oauth/token/refresh`). If you see a "not yet supported" error, fall back to `arara login` with an API key — it'll still work.

Once the backend ships these endpoints, you can also manage active sessions:

```bash
arara sessions list
arara sessions revoke <session-id>
```

Override the OAuth client ID via `ARARA_OAUTH_CLIENT_ID` if your tenant requires a custom one.

### Man Pages

Pre-generated man pages ship with each release (Homebrew installs them automatically). To generate them locally:

```bash
make manpages           # writes to dist/man/
man arara-doctor
```

## Development

```bash
# Clone
git clone https://github.com/ararahq/cli.git
cd arara-cli

# Build
make build

# Run
./bin/arara --help

# Test
make test          # plain
make test-race     # with race detector
make cover         # with coverage report
make lint          # golangci-lint (auto-installs)
make ci            # vet + lint + race + cover
make manpages      # generate man pages locally
```

## Architecture

```
arara-cli/
├── cmd/arara/        # Entrypoint
├── internal/
│   ├── api/          # HTTP client for Arara API
│   ├── cmd/          # Cobra command definitions
│   ├── config/       # CLI configuration management
│   ├── output/       # Formatted output (tables, JSON, colors)
│   ├── tui/          # Bubble Tea interactive components
│   └── version/      # Version management
├── Makefile
└── go.mod
```

## Contributing

See [CONTRIBUTING.md](https://github.com/ararahq/.github/blob/main/CONTRIBUTING.md) for guidelines.

## License

MIT - [AraraHQ](https://ararahq.com)
