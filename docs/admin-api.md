# Admin API

Admin endpoints require:

```text
Authorization: Bearer <PROXY_API_KEY>
```

Authentication failures on `/v1/*` and `/admin/*` return an OpenAI-style JSON
error envelope with HTTP 401.

## Status

`GET /admin/status` returns inferred key state and the active key index.

```bash
curl http://127.0.0.1:8080/admin/status \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "current_key_index": 1,
  "keys": [
    {
      "index": 0,
      "state": "exhausted",
      "last_429_time": "2026-06-19T11:48:29Z",
      "current": false,
      "eligible": false,
      "retry_after_seconds": 142
    },
    {
      "index": 1,
      "state": "available",
      "current": true,
      "eligible": true
    }
  ],
  "retry_exhausted_after_seconds": 300,
  "note": "Remaining usage is unavailable from opencode-go API."
}
```

Key states are inferred by this proxy:

- `unknown`: key has not yet been proven available or exhausted
- `available`: key is currently selected and not marked exhausted
- `exhausted`: key returned a quota/usage-exhausted `429`

Additional fields:

- `eligible`: whether the key may be handed out on the next request. An
  exhausted key becomes eligible again once its cooldown elapses.
- `retry_after_seconds`: remaining cooldown for an exhausted key that is not yet
  eligible. Omitted for eligible keys and when the cooldown is disabled.
- `retry_exhausted_after_seconds`: the configured cooldown before an exhausted
  key is retried automatically. `0` means the cooldown is disabled.

An exhausted key is retried automatically once its cooldown elapses; the next
real request acts as the probe. See
[Operations and security](operations.md) for details. The all-keys-exhausted
`429` also carries a `Retry-After` header pointing at the next probe window.

## Keys report (state and usage)

`GET /admin/keys` returns one entry per key with the same inferred state as
`/admin/status` plus the real usage percentages from OpenCode Go.

```bash
curl http://127.0.0.1:8080/admin/keys \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "generated_at": "2026-10-25T19:40:00Z",
  "usage_cached_seconds": 60,
  "keys": [
    {
      "index": 0,
      "name": "main",
      "state": "available",
      "current": true,
      "eligible": true,
      "usage": {
        "rolling": { "percent": 12.5, "resets_at": "2026-10-25T21:00:00Z" },
        "weekly": { "percent": 40, "resets_at": "2026-10-27T00:00:00Z" },
        "monthly": { "percent": 7, "resets_at": "2026-11-01T00:00:00Z" }
      }
    },
    {
      "index": 1,
      "name": "spare",
      "state": "exhausted",
      "last_429_time": "2026-10-25T19:30:00Z",
      "current": false,
      "eligible": false,
      "retry_after_seconds": 142,
      "usage_error": "HTTP 401"
    }
  ]
}
```

- `name` comes from `OPENCODE_GO_KEY_NAMES` (by position; empty if missing). The
  key value is never returned, logged, or included in an error: name and
  `index` identify the key.
- `state`, `current`, `eligible`, `last_429_time` and `retry_after_seconds` are
  inferred exactly as in `/admin/status` (see above).
- `usage` is fetched from the upstream by this proxy: `GET <USAGE_URL>` (default
  `https://opencode.ai/zen/go/v1/usage`) with `Authorization: Bearer <that key>`.
  The upstream answers `{"usage": {"rolling"|"weekly"|"monthly": {"percent",
  "resetsAt"}}}`; the proxy renames `resetsAt` to `resets_at` and passes its
  value through unchanged. The call does not spend quota. Windows the upstream
  omits are omitted here.
- Keys are queried in parallel, 5 second timeout each, redirects are not
  followed. If a key's lookup fails, `usage` is absent and `usage_error` holds
  only the HTTP status (`HTTP 401`) or the error type (`timeout`,
  `invalid_response`, `no_windows`, a Go type name).
- Results (including errors) are cached in memory per key for `USAGE_CACHE_TTL`
  (default 60 seconds); `usage_cached_seconds` reports that TTL. State fields are
  always live.

## Validate keys

`POST /admin/validate-keys` actively checks every configured upstream key against
`/models`, updates in-memory key state, and returns per-key validation results.

```bash
curl -X POST http://127.0.0.1:8080/admin/validate-keys \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

## Health and readiness

`GET /healthz` is unauthenticated and intended for basic health checks.

`GET /readyz` is unauthenticated. It returns JSON, verifies required config is
loaded, and checks the currently selected non-exhausted upstream key by calling
`/models` with a 5 second timeout.

## Usage and quota visibility

`/admin/status` only reports state inferred from upstream responses. For the
real usage percentages per key, use `GET /admin/keys` (above), which queries the
OpenCode Go usage endpoint.
