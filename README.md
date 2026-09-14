<p align="center">
  <img src="assets/logo.png" alt="Switchboard Go logo" width="220">
</p>

<h1 align="center">Switchboard Go</h1>

Switchboard Go is a small local proxy for the OpenCode Go API.

It gives OpenAI-compatible and Anthropic Messages-compatible tools one stable
local endpoint and automatically cycles through your upstream OpenCode Go API
keys when one is exhausted.

Most users should run it on their own computer:

```text
OpenAI/Anthropic-compatible app -> http://127.0.0.1:8080/v1 -> OpenCode Go
```

## Why use it?

- One local `/v1/*` endpoint for OpenAI-compatible and Anthropic Messages
  requests
- One proxy API key for your tools
- Multiple upstream OpenCode Go keys behind the scenes
- Automatic failover when an upstream key is exhausted
- Automatic retry of exhausted keys after a configurable cooldown, so a
  replenished account recovers without a restart or manual reset
- Optional YAML config, Docker, admin status, and SMTP alerts

## Install

Download a binary from GitHub Releases:

```text
https://github.com/ArsalanDotMe/switchboard-go/releases
```

## Quick start

```bash
export PROXY_API_KEY="replace-with-a-long-random-local-key"
export OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third"
export LISTEN_ADDR="127.0.0.1:8080"

switchboard-go
```

## Use it from an OpenAI-compatible client

Use the proxy key, not your upstream OpenCode Go key:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
```

Example request:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 100
  }'
```

## Use it from an Anthropic Messages-compatible client

Anthropic-style clients should use the same base URL and proxy key. Switchboard
Go authenticates clients with the proxy key, then forwards upstream with the
current OpenCode Go key in `x-api-key`:

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8080"
export ANTHROPIC_API_KEY="$PROXY_API_KEY"
```

Example request:

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "x-api-key: $PROXY_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-m3",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "Say hello"}]
  }'
```

For opencode and Pi Coding Agent examples, see
[docs/agent-config.md](docs/agent-config.md).

## Use it with Hermes Agent

Hermes Agent has a built-in `opencode-go` provider for OpenCode Go. Use that
provider with Switchboard Go so Hermes sends the session information OpenCode Go
needs. A generic `openai-api` provider may not send it.

Set the provider, your Switchboard Go URL, a model supported by your OpenCode Go
account, and the environment variable that will hold your Switchboard proxy
key:

```bash
hermes config set model.provider opencode-go
hermes config set model.base_url "https://switchboard.example.com/v1"
hermes config set model.default "<model-id>"
hermes config set model.key_env OPENAI_API_KEY
```

Replace the example URL with the address of your Switchboard Go instance. Set
`OPENAI_API_KEY` in the environment used to start Hermes, and give it your
Switchboard `PROXY_API_KEY` value. Do not use an upstream OpenCode Go key here.
If Hermes runs as a background gateway, make sure the gateway service can read
that environment variable.

Restart the Hermes gateway after changing its settings, then send a small test
prompt:

```bash
hermes gateway restart
hermes chat -q "Reply with exactly: switchboard ok" --oneshot
```

## Common settings

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `PROXY_API_KEY` | Yes | | Key clients use to access Switchboard Go. |
| `OPENCODE_GO_API_KEYS` | Yes | | Comma-separated upstream OpenCode Go API keys. |
| `LISTEN_ADDR` | No | `:8080` | Use `127.0.0.1:8080` for local-only access. |
| `UPSTREAM_BASE_URL` | No | `https://opencode.ai/zen/go/v1` | OpenCode Go upstream base URL. |
| `RETRY_EXHAUSTED_AFTER` | No | `5m` | Cooldown before an exhausted key is retried automatically. `0` disables it. |
| `UPSTREAM_RESPONSE_HEADER_TIMEOUT` | No | `30s` | Max wait for response headers from the upstream API. Raise this if slow LLM requests fail with a header-timeout error. `0` disables it. |

YAML config is also supported. See
[docs/configuration.md](docs/configuration.md).

## Admin endpoints

Use `Authorization: Bearer $PROXY_API_KEY`:

- `GET /admin/status`
- `POST /admin/validate-keys`

Health checks:

- `GET /healthz`
- `GET /readyz`

See [docs/admin-api.md](docs/admin-api.md).

## More docs

- [Configuration](docs/configuration.md)
- [Agent/client setup](docs/agent-config.md)
- [Admin API](docs/admin-api.md)
- [Docker](docs/docker.md)
- [systemd deployment](docs/deployment.md)
- [SMTP notifications](docs/smtp.md)
- [Operations and security](docs/operations.md)

## Development

```bash
go test ./...
gofmt -w .
go build ./...
```

## License

MIT. See [LICENSE](LICENSE).
