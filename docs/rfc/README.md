# RFCs

Substantial changes to `arara-cli` go through a lightweight RFC ("Request for Comments") process. The goal is **deliberation in public**: capture the trade-offs we considered, the alternatives we rejected, and the questions still open — so future contributors can reconstruct *why* the code looks the way it does.

This is loosely modelled on the [Rust RFC process](https://github.com/rust-lang/rfcs), trimmed to fit a small CLI project.

## When you need an RFC

You need one when the change affects:

- **Public CLI surface** — new top-level commands, persistent flags, output formats, env vars.
- **External contracts** — file formats (config, settings, hooks payloads), wire protocols (MCP, webhooks), plugin ABI.
- **Cross-cutting refactors** — anything that touches more than two packages or rewrites a critical path (auth, retry, error handling).
- **Backwards-incompatible changes** — even small ones; explain the migration story.

You **don't** need one for: bug fixes, internal refactors, dependency bumps, doc updates, new tests, or new subcommands that fit cleanly into existing patterns (e.g., adding `arara contacts export` when `arara contacts import` already exists).

When in doubt, open a draft PR — a maintainer will tell you if it warrants an RFC instead.

## Process

1. **Copy** [`0000-template.md`](./0000-template.md) to `NNNN-short-title.md`. Pick the next free number.
2. **Open a PR** with the RFC. Status starts as `Draft`.
3. **Discussion** happens in PR comments. Push commits to refine the doc.
4. **Final comment period**: when discussion settles, a maintainer marks the RFC `Final Comment Period (FCP)` and gives 7 days for objections.
5. **Decision**: `Accepted` (merged into `main`) or `Rejected` (PR closed, RFC kept for the historical record under `docs/rfc/rejected/`).
6. **Implementation**: a separate PR (or series of PRs) implements the RFC. Reference the RFC number in commits and the changelog.

The RFC author is not obligated to be the implementer.

## Statuses

| Status | Meaning |
|---|---|
| `Draft` | Author is still iterating; feedback welcome but not blocking. |
| `Active` | Open for review by maintainers and the community. |
| `Final Comment Period` | Decision pending in 7 days, voice objections now. |
| `Accepted` | Merged. Implementation can begin. |
| `Implemented` | All implementation PRs merged. RFC is now historical. |
| `Rejected` | Will not be implemented. Document why for future reference. |
| `Superseded` | Replaced by a later RFC. Link to the successor. |

## Index

| # | Title | Status | Owner |
|---|---|---|---|
| [0001](./0001-mcp-server.md) | MCP Server (`arara mcp`) | `Implemented` | TBD |
| [0002](./0002-repl-and-slash-commands.md) | REPL Stabilization + Slash Commands | `Implemented` | TBD |
| [0003](./0003-webhook-cli-listener.md) | Webhook CLI Listener (`arara listen --capture`) | `Draft` (backend pending) | Micael |
