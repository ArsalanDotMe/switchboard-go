# Configuration

Switchboard Go supports YAML config files and environment variables.

Configuration precedence, from lowest to highest:

1. Built-in defaults
2. YAML config file
3. Environment variables

This lets you keep normal settings in a config file while injecting secrets with
environment variables, systemd environment files, Docker secrets, or your
deployment platform.

## Config file discovery

Switchboard Go looks for a config file in this order:

1. `SWITCHBOARD_GO_CONFIG`, if set
2. `~/.config/switchboard-go/config.yaml`, if it exists
3. `/etc/switchboard-go/config.yaml`, if it exists
4. No config file

If `SWITCHBOARD_GO_CONFIG` is set, the file must exist and be valid YAML.
Missing or invalid explicit config files are startup errors.

Recommended user config path:

```text
~/.config/switchboard-go/config.yaml
```

Recommended system config path:

```text
/etc/switchboard-go/config.yaml
```

Use restrictive permissions, such as `0600`, for config files containing
secrets.

## YAML example

```yaml
server:
  listen_addr: "127.0.0.1:8080"
  proxy_api_key: "replace-with-a-long-random-local-key"

upstream:
  base_url: "https://opencode.ai/zen/go/v1"
  api_keys:
    - "sk-first"
    - "sk-second"
    - "sk-third"
  # How long an exhausted key waits before it is retried automatically.
  # Accepts a Go duration ("30s", "5m", "1h"). "0" disables the cooldown so
  # exhausted keys are retried on the very next request.
  retry_exhausted_after: "5m"
  # Max wait for response headers from the upstream API. LLM completions can
  # spend a long time before sending their first body bytes, so slow
  # non-streaming requests may need this raised. "0" disables the timeout.
  response_header_timeout: "30s"

smtp:
  host: "smtp.example.com"
  port: 587
  username: "alerts@example.com"
  password: "your-smtp-password"
  from: "alerts@example.com"
  to: "you@example.com"
  tls: false
  starttls: true

limits:
  max_request_body_bytes: 20971520
```

## Environment variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `SWITCHBOARD_GO_CONFIG` | No | | Explicit YAML config path. |
| `PROXY_API_KEY` | Yes\* | | API key clients must use to access this proxy. |
| `OPENCODE_GO_API_KEYS` | Yes\* | | Comma-separated OpenCode Go API keys. |
| `LISTEN_ADDR` | No | `:8080` | HTTP listen address. Use `127.0.0.1:8080` for local-only access. |
| `UPSTREAM_BASE_URL` | No | `https://opencode.ai/zen/go/v1` | OpenCode Go upstream base URL for OpenAI-compatible and Anthropic Messages-compatible routes. |
| `MAX_REQUEST_BODY_BYTES` | No | `20971520` | Maximum request body size. Requests above this return `413`. |
| `RETRY_EXHAUSTED_AFTER` | No | `5m` | Cooldown before an exhausted key is retried automatically. Go duration (`30s`, `5m`). `0` disables the cooldown (retry on the next request). Maps to `upstream.retry_exhausted_after`. |
| `UPSTREAM_RESPONSE_HEADER_TIMEOUT` | No | `30s` | Max wait for response headers from the upstream API. Go duration (`30s`, `5m`). `0` disables it. Maps to `upstream.response_header_timeout`. |
| `SMTP_HOST` | No | | SMTP host for notifications. |
| `SMTP_PORT` | No | `25` | SMTP port. |
| `SMTP_USERNAME` | No | | SMTP username. If empty, SMTP AUTH is skipped. |
| `SMTP_PASSWORD` | No | | SMTP password. |
| `SMTP_FROM` | No | | Sender email address. Required for notifications. |
| `SMTP_TO` | No | | Recipient email address. Required for notifications. |
| `SMTP_TLS` | No | `false` | Use implicit TLS when connecting to SMTP. |
| `SMTP_STARTTLS` | No | `false` | Use STARTTLS if the server advertises it. |

\* Required unless provided by YAML config.

Notifications are disabled unless `SMTP_HOST`, `SMTP_FROM`, and `SMTP_TO` are
all set.

## Validate config

Run a config-only check without starting the server:

```bash
switchboard-go validate-config
```

It loads config using normal precedence, validates required values, and prints a
safe summary.

Example env file for systemd:

```bash
PROXY_API_KEY=replace-with-a-long-random-local-key
OPENCODE_GO_API_KEYS=sk-first,sk-second,sk-third
SMTP_PASSWORD=replace-me
```
