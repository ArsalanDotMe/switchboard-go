# Deployment

Most users should run Switchboard Go locally on their own computer with:

```bash
export LISTEN_ADDR="127.0.0.1:8080"
switchboard-go
```

If you want to run it as a background service on a trusted machine, systemd is a
simple option.

## systemd example

Create `/etc/systemd/system/switchboard-go.service`:

```ini
[Unit]
Description=Switchboard Go OpenAI-compatible OpenCode Go key-cycling proxy
After=network.target

[Service]
Type=simple
Environment=SWITCHBOARD_GO_CONFIG=/etc/switchboard-go/config.yaml
EnvironmentFile=-/etc/switchboard-go/switchboard.env
ExecStart=/usr/local/bin/switchboard-go
Restart=always
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Example `/etc/switchboard-go/switchboard.env`:

```bash
PROXY_API_KEY=replace-with-a-long-random-local-key
OPENCODE_GO_API_KEYS=sk-first,sk-second,sk-third
SMTP_PASSWORD=replace-me
```

Use restrictive permissions for config and env files containing secrets.

If exposing beyond localhost or a trusted LAN, put the service behind TLS and a
firewall.
