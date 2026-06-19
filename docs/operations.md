# Operations and security

## How request forwarding works

Clients call this proxy as if it were an OpenAI-compatible API:

```text
OpenAI-compatible tool -> Switchboard Go -> https://opencode.ai/zen/go/v1
```

Incoming `/v1` paths are stripped before forwarding to the configured upstream
base URL:

```text
GET  /v1/models           -> GET  https://opencode.ai/zen/go/v1/models
POST /v1/chat/completions -> POST https://opencode.ai/zen/go/v1/chat/completions
```

When an upstream key returns a quota/usage-exhausted `429`, Switchboard Go marks
that key as exhausted, sends a best-effort SMTP notification if configured, and
retries the same request with the next non-exhausted key. If every key is
exhausted, it returns an OpenAI-style `429` JSON error.

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
  SMTP configured yes/no, config source path, and max request body bytes.
- Request bodies larger than `max_request_body_bytes` are rejected with HTTP
  `413`.
- Hop-by-hop and proxy headers are stripped when forwarding.
- The proxy sets a compatible default upstream `User-Agent` if the client does
  not provide one. This avoids upstream blocks of generic HTTP clients.
- If an upstream stream begins successfully and then later emits an error, the
  proxy does not recover mid-stream; it only retries before a response is sent.
- Exhausted keys remain exhausted until the process restarts.

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
