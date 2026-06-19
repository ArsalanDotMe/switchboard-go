# Docker

Build the image:

```bash
docker build -t switchboard-go .
```

Run with environment variables:

```bash
docker run --rm \
  -p 8080:8080 \
  -e LISTEN_ADDR=0.0.0.0:8080 \
  -e PROXY_API_KEY="replace-with-a-long-random-local-key" \
  -e OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third" \
  switchboard-go
```

Run with a mounted YAML config:

```bash
docker run --rm \
  -p 8080:8080 \
  -e SWITCHBOARD_GO_CONFIG=/config/config.yaml \
  -v "$PWD/config.yaml:/config/config.yaml:ro" \
  switchboard-go
```

Run with non-secret settings in YAML and secrets from env overrides:

```bash
docker run --rm \
  -p 8080:8080 \
  -e SWITCHBOARD_GO_CONFIG=/config/config.yaml \
  -e PROXY_API_KEY="replace-with-a-long-random-local-key" \
  -e OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third" \
  -v "$PWD/config.yaml:/config/config.yaml:ro" \
  switchboard-go
```

The Docker image uses a multi-stage Go build and a distroless non-root runtime
image.

## GHCR

The repo includes a workflow that publishes images to GHCR:

```text
ghcr.io/arsalandotme/switchboard-go
```
