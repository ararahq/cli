# RFC 0003: Webhook CLI Listener (`arara listen --capture`)

- **Status**: Draft (backend implementation pending)
- **Author(s)**: Micael Marques <micael@ararahq.com>
- **Created**: 2026-05-11
- **Updated**: 2026-05-11
- **Related**: RFC 0001 (MCP server), `internal/cmd/listen.go`, `internal/api/stream.go`, `ararahq-api/.../ClientWebhookService.kt`

## Summary

Let a developer test their AraraHQ webhook integration end-to-end on `localhost`, with no public URL, no ngrok, no dashboard reconfiguration. `arara listen --capture --forward-to http://localhost:3000/webhook` registers an ephemeral capture listener on the AraraHQ backend; while it's alive, real outbound webhook events the org would normally POST to the configured URL are mirrored down the SSE stream and POSTed to the local URL — with a valid `X-Arara-Signature` header generated from a CLI-only signing secret. Mirrors `stripe listen --forward-to`.

## Motivation

Today, a developer integrating with AraraHQ webhooks does this:

1. Spins up `ngrok http 3000` (or cloudflared, or local-tunnel).
2. Pastes the public URL into the AraraHQ dashboard `inbound_webhook_url`.
3. Triggers an event (sends a message, gets a delivery callback).
4. Reads the request in their local server.
5. Reverts the dashboard URL when done so prod traffic doesn't keep going through ngrok.

Every step here is friction. Worse, step 5 is forgotten — we've seen real customers leave their prod webhook pointing at a dead `*.ngrok.io` for days.

Stripe solved this in 2019 with `stripe listen --forward-to`. Their CLI is famous for it. Twilio still doesn't have it. Owning this for WhatsApp Business in BR is a real differentiator: zero-config local webhook testing.

The CLI side already has `arara listen --forward-to <url>` (Wave 1.3). What's missing is the backend half.

## Guide-level explanation

### Happy path

Developer building a Node service that receives AraraHQ webhooks:

```bash
$ arara listen --capture --forward-to http://localhost:3000/webhook

✔ Capture listener active (id: lst_8f3e2d, expires in 60min)
✔ Signing secret: wh_cli_4f8a2b9e3c1d (use this in your local handler)
✔ Forwarding all events for org 'acme-corp' to http://localhost:3000/webhook

> 2026-05-11T18:42:13Z message.delivered → 200 OK in 47ms
> 2026-05-11T18:42:14Z message.read → 200 OK in 22ms
^C
✔ Listener released. Webhooks resume normal delivery to the configured URL.
```

In the local handler, the developer reads `X-Arara-Signature` and verifies it against `wh_cli_4f8a2b9e3c1d`. The verification logic is **the same** they'll ship to prod — just a different secret while listening locally.

### Edge cases

**No webhook URL configured on the org.** Capture mode still works — the listener intercepts events that *would have been* sent if a URL were configured. CLI prints `(no production URL configured; capturing all events)` once at startup.

**Capture listener expires (default 60min) while CLI still running.** CLI auto-renews via heartbeat (every 30s). If the renewal POST fails 3 times in a row, CLI prints a warning and stays connected to the SSE stream in **passive** mode — events still come through, but they're now also being POSTed to the prod URL. User can re-issue the command to grab a fresh listener.

**Multiple `arara listen --capture` running for the same org.** Backend rejects the second listener with HTTP 409 + body `{error:{code:"capture_listener_active",message:"another CLI session is already capturing for this org",details:{listenerId:"lst_8f3e2d",owner:"darwin/arm64@hostname"}}}`. CLI displays the conflicting session and exits non-zero.

**Backend doesn't support capture yet.** CLI gets `404` from `POST /v1/cli/webhook-listeners`, prints a one-line warning, and falls back to **passive** SSE mode (`arara listen --forward-to` without capture). Today, that's the only mode that works.

### Errors

```bash
$ arara listen --capture --forward-to http://localhost:3000/webhook
⚠ Capture mode not yet supported by this AraraHQ environment.
⚠ Falling back to passive SSE mode — events will be forwarded but signatures
  will be missing and the production URL will continue receiving callbacks too.
> 2026-05-11T18:42:13Z message.delivered → 200 OK in 47ms
```

```bash
$ arara listen --capture --forward-to http://localhost:3000/webhook
✘ Another CLI session is already capturing for org 'acme-corp':
    listener: lst_8f3e2d
    started:  2026-05-11T18:30:00Z (12min ago)
    owner:    darwin/arm64 @ macbook-pro.local
  Stop that session before starting a new one, or wait until it expires
  (expires_at: 2026-05-11T19:30:00Z).
```

## Reference-level explanation

### New endpoints

#### `POST /v1/cli/webhook-listeners`

Create an ephemeral capture listener for the org of the authenticated user.

**Request:**
```json
{
  "owner": "darwin/arm64@macbook-pro.local",
  "ttlSeconds": 3600
}
```
- `owner` (string, optional): free-form identifier for `arara org session show` and conflict messages. CLI sends `runtime.GOOS/runtime.GOARCH@hostname`. ≤ 128 chars.
- `ttlSeconds` (int, optional, default 3600, max 14400): how long the listener lives without heartbeat.

**Response 201:**
```json
{
  "id": "lst_8f3e2d",
  "secret": "wh_cli_4f8a2b9e3c1d6075abcdef0123456789",
  "organizationId": "org_acme",
  "expiresAt": "2026-05-11T19:30:00Z",
  "heartbeatIntervalSeconds": 30
}
```
- `secret` is shown **once** in this response and used by the backend to sign forwarded events. CLI displays it to the user but does NOT persist it (memory-only, dies with the process).
- `heartbeatIntervalSeconds` lets the backend tune CLI's heartbeat cadence.

**Response 409 — listener already active:**
```json
{
  "error": {
    "code": "capture_listener_active",
    "message": "another CLI session is already capturing for this org",
    "details": {
      "listenerId": "lst_8f3e2d",
      "owner": "darwin/arm64@macbook-pro.local",
      "startedAt": "2026-05-11T18:30:00Z",
      "expiresAt": "2026-05-11T19:30:00Z"
    }
  }
}
```

**Response 404 — feature flag off / not deployed:** standard 404 envelope. CLI uses this to fall back to passive mode.

#### `POST /v1/cli/webhook-listeners/{id}/heartbeat`

**Request:** empty body.
**Response 204** on success. **Response 410** if the listener already expired/was released. **Response 403** if the API key doesn't own the listener.

#### `DELETE /v1/cli/webhook-listeners/{id}`

Releases the listener early. CLI calls this on graceful exit (SIGINT). **Response 204**. Idempotent.

#### `GET /v1/cli/webhook-listeners` (optional, nice-to-have)

List active listeners for the org. Useful for `arara org session show` and for users debugging "why is my dashboard URL still receiving zero traffic" (answer: a forgotten CLI capture is intercepting).

### Stream envelope changes

Today the SSE stream's `data:` line carries the raw event JSON. To support signature propagation **without breaking existing consumers**, the backend wraps each event in an envelope when streaming to a CLI client:

```
event: message.delivered
data: {"event":"message.delivered","data":{...real payload...},"signature":"sha256=abc123..."}
```

The CLI parser detects the envelope (presence of all three keys: `event`, `data`, `signature`) and:
- uses `signature` as the `X-Arara-Signature` header on the `--forward-to` POST
- uses `data` as the request body (so the local handler sees the same payload prod would see)
- ignores the outer `event` field (already known from the SSE `event:` line)

For backwards compatibility, if `data:` is not an envelope (no `signature` key), the parser treats the entire `data:` content as the body and skips the signature header. This means the new envelope is opt-in per-event on the backend side — old clients still work.

### Modification to `ClientWebhookService.sendEvent`

Pseudocode delta in `ararahq-api`:

```kotlin
fun sendEvent(organization: Organization, event: String, data: Map<String, Any?>) {
    val activeListener = cliListenerRepository.findActiveForOrg(organization.id!!)

    if (activeListener != null) {
        // Capture mode: emit to SSE stream with the listener's secret signature,
        // and SKIP the normal HTTP delivery to organization.inboundWebhookUrl.
        val payloadJson = objectMapper.writeValueAsString(buildPayload(event, data, organization))
        val signature = computeSignature(activeListener.secret, payloadJson)
        streamPublisher.publish(organization.id, SseEnvelope(
            event = event,
            data = parsePayload(payloadJson),
            signature = "sha256=$signature"
        ))
        return  // ← does not persist WebhookDelivery, does not POST prod URL
    }

    // ... existing behavior unchanged
}
```

Backend operators can disable this behavior per-org via `organization.allow_cli_capture` (default `true`) — useful for prod orgs that want to refuse capture mode entirely.

### Schema migration

```sql
-- Flyway V{N}__create_cli_webhook_listeners.sql
CREATE TABLE IF NOT EXISTS cli_webhook_listeners (
  id                            VARCHAR(32) PRIMARY KEY,
  organization_id               UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  secret_hash                   VARCHAR(128) NOT NULL,        -- bcrypt of the secret; raw value never persisted
  owner                         VARCHAR(128),
  created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at                    TIMESTAMPTZ NOT NULL,
  last_heartbeat_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  released_at                   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_cli_listeners_active ON cli_webhook_listeners(organization_id, released_at, expires_at);
ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS allow_cli_capture BOOLEAN NOT NULL DEFAULT TRUE;
```

- `secret_hash` is bcrypt — backend can verify listener-owned signatures during stream emission, but a leaked DB dump doesn't leak active capture secrets.
- A scheduled job (every 60s) marks listeners with `expires_at < NOW() AND released_at IS NULL` as expired and cleans heartbeats older than 5min.
- Single-active-listener-per-org constraint enforced at app layer (race window is small and `409` is acceptable UX).

### Affected packages

**Backend (`ararahq-api`):**
- `controller/CliWebhookListenerController.kt` (new)
- `services/CliWebhookListenerService.kt` (new)
- `services/ClientWebhookService.kt` (modified — capture-aware)
- `services/StreamPublisher.kt` (new — pub/sub for SSE; today missing entirely)
- `controller/StreamController.kt` (new — implements `/v1/stream` for real)
- `models/domain/CliWebhookListener.kt` (new entity)
- `models/repository/CliWebhookListenerRepository.kt` (new)
- Flyway: `V{N}__create_cli_webhook_listeners.sql`

**CLI (`arara-cli`):**
- `internal/api/stream.go` — SSEEvent gains `Signature` field; parser detects envelope.
- `internal/api/webhook_listener.go` (new) — `CreateWebhookListener`, `HeartbeatWebhookListener`, `ReleaseWebhookListener`.
- `internal/cmd/listen.go` — `--capture` flag, capture lifecycle (create → heartbeat goroutine → defer release), forwarder propagates signature header.

### Test plan

**Backend:**
- Unit: `CliWebhookListenerService` create/heartbeat/release, single-active enforcement, secret hashing.
- Integration: end-to-end via `MockMvc` — create listener, send a fake event through the publisher, assert SSE consumer receives envelope with valid signature, assert no `WebhookDelivery` row was created.
- Migration test: apply Flyway + assert column/index exist + downgrade is non-destructive.

**CLI:**
- `internal/api/stream_test.go` — envelope detection, backwards compat with raw `data:`, signature passthrough.
- `internal/cmd/listen_test.go` — `--capture` + 200 → captures secret + heartbeat; `--capture` + 404 → fallback message + passive mode continues; `--capture` + 409 → exits non-zero with helpful conflict message.
- E2E (when staging available): real backend, assert local POST receives signed body verifiable against returned secret.

### Security implications

- The capture secret is exposed in the CLI terminal output. We mark it with `wh_cli_` prefix so it's distinguishable from prod webhook secrets and easy to grep/rotate.
- A capture session bypasses the org's configured webhook URL entirely. Prod traffic stops receiving callbacks for the duration. This is intentional (matches Stripe) but is a foot-gun for someone who runs `arara listen --capture` against the wrong profile. Mitigations:
  - CLI prints active org slug + name in the startup banner; if `live` mode, prints in red and adds a 3-second confirmation prompt unless `--yes`.
  - `arara org session show` (planned) lists active capture sessions.
  - Backend optionally enforces `organization.allow_cli_capture = false` for prod-only orgs.
- The HMAC algorithm and prefix format must match what `ClientWebhookService.computeSignature` produces today (`X-Arara-Signature: sha256=<hex>`). Any divergence = silent breakage in customer apps.

### Performance implications

- One DB row per active capture session. Negligible.
- Stream publisher introduces a fanout layer (events × subscribers per org). For an org with one CLI session listening + zero other subscribers, this is a single in-memory channel write per event — cheap.
- Backend memory: each active SSE connection holds a buffered channel (~64 events). 1000 simultaneous capture sessions = ~5MB. Bound by an `organization.max_cli_listeners` cap (default 1, see single-active enforcement above).

## Drawbacks

- **Production webhook URL is silenced during capture.** A user who forgets they have `arara listen --capture` running, or who mistargets the wrong org, will see "production webhooks stopped working" symptoms. Mitigation: TTL of 60min default, heartbeat keeps it alive only while the CLI process is, and `arara org session show` makes it observable. Stripe accepts this trade-off; we do too.
- **Adds a stateful surface to the backend.** Today the API is mostly stateless request/response. This RFC introduces a long-lived per-org capture state. We need a janitor job + monitoring on listener counts.
- **Couples CLI release cadence to backend release cadence.** Once `arara listen --capture` is in a stable release, breaking the listener API is a CLI compat problem.

## Rationale and alternatives

- **Alternative A: ship CLI-side proxy that uses ngrok under the hood.** Rejected — depends on a third-party tunnel service, requires their account, and many corporate networks block ngrok domains.
- **Alternative B: dual-fire (capture + prod URL both receive).** Rejected — primary use case is local development where the user wants the events to *not* hit prod (avoid double-charging metrics, avoid prod handler running with dev data).
- **Alternative C: don't ship capture mode, only ship passive `--forward-to` over SSE.** Acceptable as v1 (this is roughly what we have today); rejected as long-term because (a) it requires the user to configure a public URL in the dashboard anyway, defeating the purpose; (b) signatures don't match prod.
- **Do nothing:** users keep using ngrok + dashboard juggling. Twilio CLI continues to look comparable to ours on this dimension. We lose a real differentiator.

## Prior art

- **Stripe CLI** `stripe listen --forward-to`: the canonical implementation. They distribute a `whsec_xxx` valid only for the listener's lifetime, and the CLI prints it on startup. Our `wh_cli_` prefix mirrors their `whsec_` namespacing on purpose.
- **Twilio CLI**: no equivalent. They have `twilio profiles:create` and `twilio api:` but no webhook-forwarding mode. `twilio phone-numbers:update --sms-url` requires a public URL.
- **GitHub `gh`**: no webhook listener (different domain — gh consumes the GitHub webhook from the receiver side, doesn't help you build one).
- **ngrok / cloudflared**: the third-party tunnel approach. Not bad, but adds a dependency, an account, a brand the user has to trust, and bandwidth charges at scale.
- **localtunnel / serveo**: open-source alternatives to ngrok with similar trade-offs.

## Unresolved questions

- [ ] Should the CLI silently re-create an expired listener if heartbeat fails 3×, or hard-fail and exit? (Lean: hard-fail. Silent recreation hides a real problem.)
- [ ] Do we want `--capture --replay` to also re-emit the last N missed events on connect? (Lean: defer to a follow-up RFC.)
- [ ] How do we surface the active capture session in the AraraHQ dashboard so a non-CLI user knows why their webhooks went quiet? (Lean: small banner "1 CLI session is intercepting webhooks for testing — release at <link>".)
- [ ] Should `allow_cli_capture` default to `false` for orgs in `live` mode after some maturity period? (Lean: keep default `true`, add per-org toggle, opt-out via config.)

## Future possibilities

- **`arara listen --replay-from <event-id>`**: re-fire a specific historical webhook to localhost. Useful for "I had a bug yesterday at 3am, let me debug it now."
- **`arara webhook test <event-type>`**: dispatch a synthetic event of the given type. Mirrors `stripe trigger`.
- **`arara webhook fixtures`**: load a YAML scenario file that creates contacts + sends + waits + asserts deliveries. Mirrors `stripe fixtures`.
- **WebSocket alternative to SSE**: SSE is one-way and bound to HTTP/1.1 keepalive limits in some proxies. WS would let CLI ack each event, enabling exactly-once semantics. Defer until SSE proves insufficient under real load.
