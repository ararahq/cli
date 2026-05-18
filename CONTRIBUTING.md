# Contributing to `arara-cli`

Thanks for considering a contribution. This document is the contract between the project and people sending pull requests; the goal is to make it predictable what gets merged and why.

## Quick start

```bash
git clone https://github.com/ararahq/cli.git
cd arara-cli

# Requires Go 1.26+
make build       # builds ./bin/arara
make test        # plain test suite
make test-race   # with the race detector
make cover       # coverage report
make lint        # auto-installs golangci-lint on first run
make ci          # vet + lint + race + cover (matches CI)
```

Open an issue or draft PR before sinking serious time into anything substantial — see [When you need an RFC](#when-you-need-an-rfc) below.

## How decisions are made

We use a lightweight **RFC process** for changes that affect the public surface (commands, flags, file formats, plugin/hook ABI, the MCP tool surface, anything backwards-incompatible). See [`docs/rfc/README.md`](docs/rfc/README.md) for the workflow and [`docs/rfc/0001-mcp-server.md`](docs/rfc/0001-mcp-server.md) as a worked example.

You don't need an RFC for: bug fixes, internal refactors, dependency bumps, doc tweaks, new tests, or new subcommands that fit cleanly into existing patterns (e.g., adding `arara contacts export` when `arara contacts import` already exists).

When in doubt, open a draft PR — a maintainer will tell you if it warrants an RFC instead.

## What we look for in a PR

Every PR — yours or a maintainer's — has to clear the same bar:

- [ ] **Builds**: `go build ./...` clean.
- [ ] **Tests pass under the race detector**: `make test-race`.
- [ ] **Patch coverage ≥ 90%** on changed lines. New `internal/` code should land with tests in the same PR.
- [ ] **Lint clean** on touched files: `make lint`. Pre-existing warnings in unrelated files are not your job, but don't introduce new ones.
- [ ] **No dead code, no `TODO: later`, no commented-out blocks.** Half-finished work goes into a follow-up PR, not main.
- [ ] **Constants over magic numbers**, descriptive names, early returns, short functions.
- [ ] **No new dependencies** without justification in the PR description. Re-use what's already in `go.mod` when reasonable.
- [ ] **No backwards-incompatible change** without an RFC. Renaming a flag or changing an output format counts.
- [ ] **Documentation updated** if you changed user-visible behavior (README, command help text, RFC status).
- [ ] **Changelog entry** for user-visible changes (`CHANGELOG.md`, under `## [Unreleased]`).

The PR template (auto-filled on GitHub) is the same checklist in machine-checkable form.

## Commit messages

One short line. `type(scope): description`. Imperative mood, present tense, no period.

Allowed types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `style`, `db`, `revert`, `ci`.

Examples:

```
feat(mcp): add arara_estimate_campaign tool
fix(api): retry POSTs reuse Idempotency-Key across attempts
refactor(repl): extract input parser into pure module
docs(rfc): mark 0001 implemented
ci: bump golangci-lint to v1.65
```

Don't use emoji. Don't add co-author footers. Don't write a body unless the *why* genuinely needs explaining — and if it does, keep it under 5 lines.

### Sign-off (DCO)

We use the [Developer Certificate of Origin](https://developercertificate.org/) — a one-line "I have the right to contribute this" certification. Sign your commits with `-s`:

```bash
git commit -s -m "fix(api): propagate Retry-After on 429"
```

This appends a `Signed-off-by: Your Name <you@example.com>` trailer. PRs without sign-off get bounced by a bot.

## Branch naming

Convention: `<type>/<short-slug>`. Examples:

```
feat/oauth-pkce
fix/dry-run-empty-vars
refactor/api-client-test-hooks
docs/contributing
```

Long-lived branches (`main`) are protected; you can't push to them directly. Open a PR.

## Code style

`gofmt -s` is the source of truth. CI rejects unformatted code. Beyond that:

- **Errors**: wrap with `fmt.Errorf("context: %w", err)` when adding context, sentinel `errors.New("...")` otherwise. No string-matching errors — use `errors.Is`/`errors.As` against typed sentinels.
- **Naming**: descriptive over short. `validationError` beats `err`. `parseError` beats `e`. The variable names in this codebase are deliberately longer than typical Go — match the style.
- **No `interface{}`** when a concrete type works. `any` is fine where the type genuinely varies (JSON payloads, MCP tool args).
- **Tests live next to the code** (`foo.go` → `foo_test.go`), package `foo` (white-box) by default. Use `foo_test` (black-box) when you want to test the public API surface from the outside.
- **Comments**: only when they explain *why*, not *what*. Don't restate the code. Function names should make the *what* obvious.

## Testing standards

This is a CLI used in production by people sending money-cost messages. Tests are not optional.

- **Pure logic** (parsers, normalizers, classifiers): table-driven unit tests, 100% coverage.
- **API client paths**: `httptest.Server`-backed tests, asserting requests AND responses (headers, body, retry behavior).
- **Filesystem/IO paths**: `t.TempDir()` + `t.Setenv("HOME", ...)`, never write to the user's real `~`.
- **External processes** (hooks, plugins, slash command execution): use real shell scripts in a temp dir, skip on Windows where appropriate via `runtime.GOOS == "windows"` guards.
- **TUI code**: out of scope for unit testing in this codebase today — extract pure logic and test that instead. We'll add `teatest`-based integration tests in a dedicated sprint.

If you find yourself reaching for `time.Sleep` in a test, stop. Inject a fake clock (`Now func() time.Time`) instead — there are existing examples in `internal/api/client.go` and `internal/mcp/server.go`.

## When you need an RFC

Open one (or wait to be asked for one) when the change touches:

- **Public CLI surface**: new top-level commands, persistent flags, output formats, env var conventions.
- **External contracts**: file formats (config, settings, hooks payloads, slash command frontmatter), wire protocols (MCP tool schemas, webhook envelope), plugin ABI.
- **Cross-cutting refactors**: rewrites of auth, retry, error handling, or anything touching more than two packages.
- **Backwards-incompatible** anything.

The template is in [`docs/rfc/0000-template.md`](docs/rfc/0000-template.md). Copy it, pick the next number, open a PR. Discussion happens in PR comments.

## Releasing

Maintainers handle releases. The flow:

1. Bump version in `CHANGELOG.md` under a new `## [vX.Y.Z]` heading.
2. Move all `## [Unreleased]` entries into the new section.
3. Tag: `git tag -s vX.Y.Z -m "vX.Y.Z"` (signed annotated tag).
4. Push: `git push origin vX.Y.Z` — triggers `goreleaser` via GitHub Actions to build cross-platform binaries, generate man pages, publish to Homebrew tap, and create the GitHub release.

If you want to propose a release, comment on the most recent `## [Unreleased]` block in a PR or open an issue.

## Reporting security issues

**Do not** open a public issue for security problems. See [SECURITY.md](SECURITY.md) for the responsible disclosure process.

## Code of Conduct

Participation in this project is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Tldr: be the colleague you'd want on the worst day of your week.
