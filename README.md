<p align="center">
  <img src="assets/logo.png" alt="Switchboard Go logo" width="220">
</p>

<h1 align="center">Switchboard Go</h1>

Switchboard Go is a small local proxy for the OpenCode Go API.

It gives OpenAI-compatible tools one stable local endpoint and automatically
cycles through your upstream OpenCode Go API keys when one is exhausted.

Most users should run it on their own computer:

```text
OpenAI-compatible app -> http://127.0.0.1:8080/v1 -> OpenCode Go
```

## Why use it?

- One local OpenAI-compatible `/v1/*` endpoint
- One proxy API key for your tools
- Multiple upstream OpenCode Go keys behind the scenes
- Automatic failover when an upstream key is exhausted
- Optional YAML config, Docker, admin status, and SMTP alerts

## Install

Download a binary from GitHub Releases:

```text
https://github.com/ArsalanDotMe/switchboard-go/releases
```

Linux x86_64 example:

```bash
curl -LO https://github.com/ArsalanDotMe/switchboard-go/releases/download/v1.0.0/switchboard-go_Linux_x86_64.tar.gz
curl -LO https://github.com/ArsalanDotMe/switchboard-go/releases/download/v1.0.0/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf switchboard-go_Linux_x86_64.tar.gz
install -m 0755 switchboard-go /usr/local/bin/switchboard-go
```

Or build from source:

```bash
go build -o switchboard-go .
```

## Quick start

```bash
export PROXY_API_KEY="replace-with-a-long-random-local-key"
export OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third"
export LISTEN_ADDR="127.0.0.1:8080"

switchboard-go
```

In another terminal:

```bash
curl http://127.0.0.1:8080/healthz

curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $PROXY_API_KEY"
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
    "model": "minimax-m3",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 100
  }'
```

For opencode and Pi Coding Agent examples, see
[docs/agent-config.md](docs/agent-config.md).

## Common settings

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `PROXY_API_KEY` | Yes | | Key clients use to access Switchboard Go. |
| `OPENCODE_GO_API_KEYS` | Yes | | Comma-separated upstream OpenCode Go API keys. |
| `LISTEN_ADDR` | No | `:8080` | Use `127.0.0.1:8080` for local-only access. |
| `UPSTREAM_BASE_URL` | No | `https://opencode.ai/zen/go/v1` | OpenCode Go upstream base URL. |

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
