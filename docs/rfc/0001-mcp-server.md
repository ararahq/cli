# RFC 0001: MCP Server (`arara mcp`)

- **Status**: Implemented
- **Author(s)**: TBD
- **Created**: 2026-05-10
- **Updated**: 2026-05-10
- **Related**: Sprint 3 retrospective; AraraHQ public API
- **Implementation**: `internal/mcp/`, `internal/cmd/mcp.go`

## Summary

Add a new subcommand `arara mcp` that runs the CLI as a [Model Context Protocol](https://modelcontextprotocol.io) server, exposing a curated subset of the AraraHQ API as MCP tools. This lets coding agents (Claude Code, Cursor, Continue, custom agents) drive WhatsApp messaging operations through the same authenticated CLI session a human would use, without re-implementing auth, retry, idempotency, or rate-limiting.

## Motivation

The CLI already has the parts an agent would need: authenticated HTTP client with retry + jitter + idempotency keys (Sprint 1), structured error taxonomy with `IsAuthError`/`IsNotFoundError`, and a clear domain model (`SendMessageRequest`, `Template`, `CampaignResponse`). Today these are only callable from a human terminal. Two patterns we keep seeing:

1. **Agents shell out via `os/exec`**, parse `arara ... -o json` stdout, and re-implement error handling. Brittle: error envelopes change, JSON shape drifts, exit codes leak through.
2. **Agents call the AraraHQ API directly** and re-implement everything we already built (keyring, retry, idempotency). Worse: every team writes their own version with different bugs.

MCP gives us a third option: `arara mcp` speaks the protocol over stdio, the agent connects once, and the agent gets typed tool calls with structured return values. Same binary, two modes (human via subcommands, agent via MCP). This is the "platform" leap that distinguishes a CLI from a *runtime*.

## Guide-level explanation

### From the agent author's perspective

Add `arara` as an MCP server in your agent's config. For Claude Code:

```json
// ~/.claude/mcp_servers.json
{
  "arara": {
    "command": "arara",
    "args": ["mcp"],
    "env": {
      "ARARA_PROFILE": "production"
    }
  }
}
```

The agent now sees a curated tool surface:

- `arara_send_message` — send a WhatsApp message (template or freeform)
- `arara_get_message_status` — check delivery state by message ID
- `arara_list_templates` — list approved templates with categories
- `arara_list_contacts` — paginated contact lookup
- `arara_get_metrics` — wallet balance, send volume, delivery rate
- `arara_estimate_campaign` — pre-compute cost for a recipient count + template

Each tool maps 1:1 to a CLI subcommand or API call we already ship.

### Example tool call

The agent calls `arara_send_message` with:

```json
{
  "to": "+5511999999999",
  "template": "appointment_reminder",
  "variables": ["John", "Wed 14:00"],
  "dryRun": true
}
```

Server returns:

```json
{
  "ok": true,
  "data": {
    "method": "POST",
    "path": "/v1/messages",
    "payload": { "...": "..." },
    "estimatedCost": { "totalCost": 0.063, "templateCategory": "UTILITY" }
  }
}
```

The agent inspects the cost, decides whether to proceed, and re-calls without `dryRun: true`. Identical UX to a human running `arara send --dry-run` first.

### Error contract

Tool errors follow the same `*APIError` envelope the CLI already exposes, but boxed into MCP's error format:

```json
{
  "ok": false,
  "error": {
    "code": "ararahq-63016",
    "message": "Janela de 24h fechada. Envie um template primeiro.",
    "retryable": false,
    "retryAfter": 0
  }
}
```

Agents that already know our error codes can branch on `code`. Agents that don't can fall back on `message`.

## Reference-level explanation

### Architecture

```
┌──────────────────┐     stdio (JSON-RPC 2.0)     ┌────────────────────┐
│  Agent (Claude,  │ ───────────────────────────► │  arara mcp         │
│  Cursor, etc.)   │ ◄─────────────────────────── │                    │
└──────────────────┘                              │  ├─ tool registry  │
                                                  │  ├─ auth (keyring) │
                                                  │  └─ api.Client     │
                                                  └────────────────────┘
                                                            │
                                                            ▼
                                                  api.ararahq.com
```

`arara mcp` is a long-lived process that:

1. Reads JSON-RPC requests from stdin.
2. Loads credentials from the keyring (same as `api.NewClientFromConfig`).
3. Dispatches to handlers that wrap `*api.Client` methods.
4. Writes JSON-RPC responses to stdout.

Logs go to stderr (never stdout — stdout is reserved for the protocol).

### Affected packages

```
internal/mcp/
├── server.go       # MCP server lifecycle (init, list_tools, call_tool, shutdown)
├── tools.go        # Tool registry: name → schema → handler
├── handlers.go     # Per-tool handlers (thin wrappers around api.Client)
├── errors.go       # APIError → MCP error mapping
└── *_test.go       # In-memory transport tests
internal/cmd/mcp.go # `arara mcp` subcommand
```

### Tool registry shape

```go
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage // JSON Schema describing args
    Handler     func(ctx context.Context, args json.RawMessage) (any, error)
    Mutating    bool // if true, requires explicit confirmation contract (see below)
}
```

### Initial tool surface (proposed)

Read tools (low risk, ship first):

| Tool | Wraps | Mutating |
|---|---|---|
| `arara_list_templates` | `Client.ListTemplates` | no |
| `arara_get_template_status` | `Client.GetTemplateStatus` | no |
| `arara_get_message_status` | `Client.GetMessageStatus` | no |
| `arara_list_contacts` | `Client.ListContacts` | no |
| `arara_get_contact_stats` | `Client.GetContactStats` | no |
| `arara_get_metrics` | `Client.GetMetrics` | no |
| `arara_get_wallet_balance` | `Client.GetWalletBalance` | no |
| `arara_list_numbers` | `Client.ListNumbers` | no |
| `arara_estimate_campaign` | `Client.EstimateCampaign` | no |

Write tools (Phase 2, behind explicit setting):

| Tool | Wraps | Mutating |
|---|---|---|
| `arara_send_message` | `Client.SendMessage` | yes |
| `arara_create_campaign` | `Client.CreateCampaign` | yes |
| `arara_import_contacts` | `Client.ImportContacts` | yes |

### Auth model

The MCP server reuses the user's existing CLI keyring credential. Two consequences:

- **Same identity, same audit trail**: server logs show "user X did Y" whether the agent or the human triggered it.
- **Same blast radius**: an agent has the *full* permissions of the credential. We mitigate this with the *Write tools opt-in* below.

`ARARA_PROFILE`, `ARARA_MODE`, `ARARA_OUTPUT` environment variables (already supported) work in MCP context, so agents can scope themselves to a `sandbox` profile.

### Write tools opt-in

Write tools are **disabled by default**. Two opt-in mechanisms:

1. **Settings flag**: `arara settings set mcp.allowWriteTools true` (user scope).
2. **CLI flag**: `arara mcp --allow-write-tools` (per-invocation, beats setting).

When write tools are disabled, `list_tools` returns only the read set. When enabled, the server's `init` response includes:

```json
{
  "capabilities": {
    "tools": { "listChanged": false },
    "experimental": {
      "araraWriteTools": true
    }
  }
}
```

The agent's responsibility from there on is to ask the human before calling them. We won't try to enforce this on the server side — agents that don't ask are buggy agents, not malicious ones, and the credential's rate-limit is the real backstop.

### Idempotency

Write tools auto-generate an `Idempotency-Key` per call (same logic as Sprint 1). The agent can supply its own via the `idempotencyKey` argument if it wants retry-safe semantics across MCP reconnects. We recommend agents always pass one based on their request UUID.

### Transport: stdio first

`arara mcp` speaks JSON-RPC over stdio (Claude Code's default and the most portable choice). HTTP/SSE transport is out of scope for v1; we'll revisit when there's a concrete user (e.g., a hosted agent that doesn't run on the same host as the CLI).

### Test plan

- **Unit**: each handler tested with a fake `api.Client` (httptest backed).
- **Protocol conformance**: JSON-RPC framing, init handshake, tool listing, tool call, error responses. Use the MCP spec's example sequences as fixtures.
- **End-to-end**: spawn `arara mcp` as a subprocess, send a sequence of init → list_tools → call_tool, assert responses. Bash + `jq` is enough for smoke tests; we can add Go integration tests if value is clear.
- **Error mapping**: every distinct `*APIError` shape (envelope, flat, raw) round-trips through MCP correctly.

### Observability

When `--verbose`/`ARARA_VERBOSE=1`, log every tool call to stderr with:

```
[mcp] tool=arara_send_message duration=312ms status=ok agent=claude-code/1.4.2
```

No payload contents (PII risk), only metadata. Verbose mode never logs to stdout (would corrupt the JSON-RPC stream).

## Drawbacks

1. **New external contract**: once we ship `arara_send_message` as a stable tool, renaming it breaks every agent config in the wild. We need a deprecation policy from day one (proposed below).
2. **Auth model is coarse**: an agent gets the user's full credential. Real least-privilege would require per-tool-call session tokens — the AraraHQ backend doesn't ship that yet (see RFC 0003 if filed).
3. **Maintenance overhead**: every new domain endpoint needs a tool definition + JSON Schema + handler + tests. We accept this as the cost of a stable public surface; the alternative (agents calling raw HTTP) just shifts the cost to consumers.
4. **MCP spec is young**: the protocol may evolve in breaking ways. We pin to a tested SDK version and bump deliberately.
5. **Stdio-only is limiting**: agents on different hosts can't connect. Acceptable for v1; HTTP/SSE is a known follow-up.

## Rationale and alternatives

### SDK choice

| Option | Pros | Cons |
|---|---|---|
| **`mark3labs/mcp-go`** *(recommended)* | Mature-enough community SDK, idiomatic Go, covers init/list/call/shutdown. Already used by several MCP servers. | Third-party dep we'd have to track. License: MIT — compatible. |
| **Custom implementation** | No external dep; we control supply chain entirely. | ~500 LOC of protocol plumbing per side; we'd be re-implementing what mark3labs already tested. |
| **Wrap an HTTP MCP gateway** | Reuse existing TS reference server. | Adds a Node.js dependency to a Go CLI — terrible install story. Rejected. |

**Recommendation**: vendor `mark3labs/mcp-go`. If supply-chain risk becomes a real concern post-launch, we can fork or reimplement; the API surface is small.

### Tool granularity

| Option | Pros | Cons |
|---|---|---|
| **One tool per CLI subcommand** *(recommended)* | Mirror is easy to explain; agents can introspect by listing CLI commands. | Slight verbosity. |
| **Generic `arara_run` taking a command string** | One tool, trivial to add new commands. | Agents lose typed args; tool listing tells them nothing useful; security worse (effectively shell exec). Rejected. |
| **One tool per HTTP endpoint** | Maximum granularity. | Internal API is volatile; we don't want to leak its shape. Rejected. |

### Naming

| Option | Pros | Cons |
|---|---|---|
| **`arara_<verb>_<resource>`** *(recommended)* | Searchable, prefix-scoped (avoids collisions in agents that load multiple servers). | Slightly verbose. |
| **`<verb>_<resource>`** | Cleaner. | Collides with other servers' tools (`send_message` from a different MCP server would conflict). |

### Do nothing

If we don't ship MCP, agents will keep shelling out. That's *fine* for read paths (`-o json` is stable enough) but breaks down for write paths where idempotency, retries, and structured errors actually matter. The longer we wait, the more brittle integrations accumulate that we'll have to migrate later.

## Prior art

- **Stripe CLI**: doesn't ship MCP yet; agents shell out to `stripe trigger ...` — same brittleness we want to avoid.
- **GitHub CLI (`gh`)**: doesn't ship MCP; the [github-mcp-server](https://github.com/github/github-mcp-server) project is a separate repo. Splitting like this is an option but we'd lose code reuse with our existing `*api.Client`.
- **Claude Code**: ships its own MCP integration as the canonical client. Following its conventions (stdio-first, tool naming, error shape) gives us free interoperability.
- **`mcp-server-everything`**: reference TypeScript MCP server; useful as a behavioural fixture for protocol conformance tests.

## Unresolved questions

- [ ] **Q1**: Do we ship write tools in v1 or wait for a server-side scoped-token concept (RFC TBD on backend)? Recommendation: ship behind `--allow-write-tools` flag, document the credential blast radius clearly. Real least-privilege follows when backend is ready.
- [ ] **Q2**: SDK: vendor `mark3labs/mcp-go` or wait for an Anthropic-official Go SDK if/when one ships? Recommendation: vendor now, migrate later if an official SDK appears.
- [ ] **Q3**: Tool naming convention — `arara_send_message` vs `send_message` (prefix vs no prefix)? Recommendation: prefix.
- [ ] **Q4**: How do we version the tool surface? Strictly additive forever, or do we allow renames with a 2-release deprecation window? Recommendation: additive only for v1; renames require an RFC.
- [ ] **Q5**: Should `arara mcp` advertise resources (read-only documents the agent can fetch) in addition to tools? E.g., expose the user's templates as resources so agents can include them in context. Defer to a follow-up RFC.
- [ ] **Q6**: Telemetry on tool calls — do we log to user's `arara doctor` bundle, send anonymized counts to AraraHQ, both, neither? Recommendation: stderr-only logs, opt-in metrics post-v1.
- [ ] **Q7**: Confirmation gates — should write tools require an interactive confirmation step (effectively making them two-step calls)? MCP doesn't have a great primitive for this. Defer until we have a real complaint.

## Future possibilities

- **HTTP/SSE transport** for hosted agents (`arara mcp serve --bind 127.0.0.1:8765`).
- **MCP resources**: expose templates, contacts, recent messages as readable resources so agents can RAG over them without tool calls.
- **MCP prompts**: ship pre-built prompts for common workflows ("draft a cancellation message", "diagnose why a campaign is stuck").
- **Per-tool scopes** once the backend supports scoped tokens — let an agent get only `messages:send` rather than full account access.
- **Plugin tools**: let `arara-foo` plugins register their own MCP tools, surfacing them through `arara mcp` automatically. Needs a stable plugin ↔ MCP bridge protocol.
