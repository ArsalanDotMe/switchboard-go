# Operations and security

## How request forwarding works

Clients call this proxy as if it were an OpenAI-compatible API or Anthropic
Messages-compatible API:

```text
OpenAI/Anthropic-compatible tool -> Switchboard Go -> https://opencode.ai/zen/go/v1
```

Incoming `/v1` paths are stripped before forwarding to the configured upstream
base URL. Root OpenAI-compatible paths are also accepted for clients that expect
their base URL to omit `/v1`:

```text
GET  /v1/models           -> GET  https://opencode.ai/zen/go/v1/models
POST /v1/chat/completions -> POST https://opencode.ai/zen/go/v1/chat/completions
POST /v1/messages         -> POST https://opencode.ai/zen/go/v1/messages
GET  /models              -> GET  https://opencode.ai/zen/go/v1/models
POST /chat/completions    -> POST https://opencode.ai/zen/go/v1/chat/completions
POST /messages            -> POST https://opencode.ai/zen/go/v1/messages
```

When an upstream key returns a quota/usage-exhausted `429`, Switchboard Go marks
that key as exhausted, sends a best-effort SMTP notification if configured, and
retries the same request with the next eligible key. If every key is exhausted,
it returns an error shaped for the request style.

### Automatic retry of exhausted keys

Exhausted keys do not stay exhausted forever. Each exhausted key becomes
eligible for an automatic retry once `retry_exhausted_after` (default 5 minutes)
has elapsed since its last quota `429`. The next real client request is the
probe: it is forwarded with the eligible key. If the upstream account was
replenished the request just succeeds and the key is marked available again; if
it is still depleted, the key is re-marked exhausted and its cooldown restarts.
The worst-case cost is one extra failed upstream round-trip per key per cooldown
window - there is no background goroutine and no synthetic traffic.

On any successful response the key is marked available and the notification
flags re-arm, so a later depletion round alerts again instead of staying silent.

While every key is within its cooldown, the proxy fast-fails locally with a
`429` instead of hammering upstream, and includes a `Retry-After` header pointing
at the next probe window so well-behaved clients (for example opencode, which
honors `Retry-After`) pace themselves. Setting `retry_exhausted_after` to `0`
disables the cooldown: exhausted keys are retried on the very next request and no
`Retry-After` header is sent, leaving client backoff as the only pacing.

## Security notes

- Treat both `PROXY_API_KEY` and `OPENCODE_GO_API_KEYS` as secrets.
- Use a long, random `PROXY_API_KEY`.
- Prefer environment variables or secret managers for credentials.
- If secrets are stored in YAML, set file permissions to `0600`.
- Bind to `127.0.0.1` unless other machines on your LAN need access.
- If exposing beyond a trusted LAN, put the service behind TLS and a firewall.
- Do not commit API keys, SMTP credentials, or systemd files containing secrets.
- `/healthz` and `/readyz` are intentionally unauthenticated for health checks.
- `/v1/*` and `/admin/*` require authentication.

## Operational behavior

- Startup logs include listen address, upstream base URL, upstream key count,
  SMTP configured yes/no, config source path, max request body bytes, and the
  exhausted-key retry cooldown.
- Request bodies larger than `max_request_body_bytes` are rejected with HTTP
  `413`.
- Hop-by-hop and proxy headers are stripped when forwarding.
- The proxy sets a compatible default upstream `User-Agent` if the client does
  not provide one. This avoids upstream blocks of generic HTTP clients.
- The proxy preserves a client-supplied `x-opencode-session` header. If it is
  absent, Switchboard derives an opaque value from `X-Hermes-Session-Id`,
  `X-Hermes-Session-Key`, or a string request-body `prompt_cache_key`,
  `session_id`, `conversation_id`, or `user`, in that order. Hermes Responses
  requests expose their logical conversation scope through `prompt_cache_key`.
  If no conversation signal is available, Switchboard falls back to a stable
  client identity derived from the client address and user agent. Raw session
  values, client addresses, and proxy credentials are never sent upstream.
  Clients should still send `x-opencode-session` directly when possible.
- OpenAI-compatible requests forward the upstream key as `Authorization:
  Bearer ...`; Anthropic Messages-compatible requests forward it as
  `x-api-key`.
- If an upstream stream begins successfully and then later emits an error, the
  proxy does not recover mid-stream; it only retries before a response is sent.
- Exhausted keys are retried automatically once `retry_exhausted_after` has
  elapsed (default 5 minutes); the admin API can still reset them immediately.

## Release artifacts

Published release assets include:

- `switchboard-go_Linux_x86_64.tar.gz`
- `switchboard-go_Linux_arm64.tar.gz`
- `switchboard-go_Darwin_x86_64.tar.gz`
- `switchboard-go_Darwin_arm64.tar.gz`
- `switchboard-go_Windows_x86_64.zip`
- `checksums.txt`

Tagged GitHub releases are built by `.github/workflows/release.yml` and uploaded
to the GitHub Release with SHA256 checksums.

You can create release artifacts locally with GoReleaser:

```bash
goreleaser release --snapshot --clean
```
