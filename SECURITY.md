# Security Policy

We treat security reports as the highest-priority issue category. This
document is the contract: how we want to be told about vulnerabilities,
what we promise back, and how the CLI handles your credentials so you can
audit its trust boundary before you adopt it.

## Reporting a Vulnerability

**Please do not open public GitHub issues for security bugs.** Use one of
the private channels below.

- **Preferred:** [GitHub Security Advisories](https://github.com/ararahq/cli/security/advisories/new)
  — encrypted, tied to the repo, lets us collaborate on a fix and a CVE.
- **Email:** `security@ararahq.com` — for issues you'd rather not file via
  GitHub. Subject line should start with `[arara-cli security]`. PGP key
  on request.

Include in your report (best effort, anything you can give helps):

- A short description of the issue and its impact.
- Steps to reproduce, plus the CLI version (`arara --version`) and OS.
- Whether you've already disclosed this anywhere else.
- How you'd like to be credited in the advisory (or "anonymous").

## Our Commitments

| When | What |
|---|---|
| Within **24 business hours** | Acknowledge receipt and assign an owner. |
| Within **5 business days** | Confirm whether it's reproducible and give a severity estimate. |
| Before public disclosure | Coordinate a fix window with you; default 90 days from confirmation, faster if there's an active exploit. |
| After fix lands | Credit you in the advisory unless you ask us not to; tag the CVE if one was assigned. |

We don't have a paid bug bounty yet — but contributions to the security
of the CLI are taken seriously and credited publicly.

## Supported Versions

We patch security fixes against the **latest minor release**. Older
minor releases get fixes on a best-effort basis only — upgrade is the
canonical path.

| Version | Supported |
|---|---|
| Latest minor (currently `0.x`) | ✅ |
| Anything older | ⚠️ best-effort |

`arara upgrade --install` updates in place, verifies the SHA256 of the
new release, and preserves your config.

## Credential Handling — Audit Trail

The CLI handles two classes of credential: **API keys** (`ara_live_…` /
`ara_test_…`) and **OAuth tokens** (returned by the Device Authorization
flow). Both are treated as bearer secrets.

### Storage

1. **OS keyring first.** macOS Keychain, Windows Credential Manager, or
   Linux Secret Service (libsecret / kwallet). The CLI calls
   `github.com/zalando/go-keyring`; we never see plaintext after the
   first write.
2. **`~/.config/arara/config.toml` fallback.** If the keyring is not
   available (CI containers, headless servers, broken libsecret), the
   key is written to the user's config file with permissions `0600`. The
   file path is logged so you know it happened — there is no silent
   fallback.
3. **No environment-variable leak.** The CLI does not read
   `ARARA_API_KEY` from `process.env` automatically. You have to opt
   in with `arara login --key "$ARARA_API_KEY"`.

### In Memory

- The `Client` holds the active token for the lifetime of the process.
- Subprocess and HTTP middleware redact tokens before logging
  (see `internal/api/redact.go`). The redactor strips by prefix
  (`ara_live_`, `ara_test_`, `Bearer …`, `wh_cli_`, `whsec_…`) and by
  field name (`token`, `secret`, `apiKey`, `authorization`,
  `password`).

### In Transit

- All API requests are HTTPS-only. There is no `--insecure` flag.
- Bearer tokens go in the `Authorization` header, never in the URL or
  query string.

### Webhook Capture Secrets (`arara listen --capture`)

When you start a CLI capture listener, the backend issues a one-shot
signing secret (`wh_cli_…`). This secret:

- Is shown **once** in the create-response and printed to the terminal.
- Lives in the CLI process memory only — **not** written to disk, the
  keyring, or anywhere else.
- Dies when the process exits. There is no recovery.

If your terminal scrollback is captured (shared session, screen
recording), treat the secret as compromised and re-run the capture
command to rotate it. Capture sessions are also bounded by TTL (default
60 min) on the backend.

## Known Risks

These are documented so they're not surprises:

- **Config file fallback** is plaintext. Use `chmod 600 ~/.config/arara/config.toml`
  if you don't trust your OS keyring (default permissions are already
  `0600`, but worth confirming on shared boxes).
- **Curl-pipe-shell install (`curl … | sh`)** trusts GitHub and the TLS
  chain. SHA256 checksums are verified against the release's
  `checksums.txt` after download. To inspect the script first:
  ```sh
  curl -fsSL https://raw.githubusercontent.com/ararahq/cli/main/install.sh > install.sh
  less install.sh
  sh ./install.sh
  ```
- **OS keyring on Linux** depends on the desktop environment. Headless
  servers will hit the file fallback unless you have `pass` or another
  Secret Service provider configured.

## Out of Scope

These are not vulnerabilities we'll act on:

- Social engineering against AraraHQ employees.
- Denial of service against `api.ararahq.com` from your own account.
- Issues that require physical access to an unlocked workstation that
  already has a logged-in CLI session.
- Self-XSS via terminal escape sequences in your own output. (We
  sanitize remote inputs; we trust the local environment.)

## Acknowledgements

Researchers who report valid issues to us in good faith:

<!-- new credits get appended here as advisories close -->
