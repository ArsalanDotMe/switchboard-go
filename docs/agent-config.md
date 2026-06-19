# Agent and client setup

Point OpenAI-compatible clients at Switchboard Go and use the proxy API key, not
your upstream OpenCode Go key.

For a local Switchboard Go process:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
```

## curl

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

Streaming requests are proxied as normal HTTP streams:

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-m3",
    "stream": true,
    "messages": [{"role": "user", "content": "Write a short poem"}]
  }'
```

## opencode

Create or edit `~/.config/opencode/opencode.json`. Preserve any existing keys
and merge in the `provider` entry plus the default `model` if desired:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "switchboard": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Switchboard",
      "options": {
        "baseURL": "http://127.0.0.1:8080/v1",
        "apiKey": "{env:SWITCHBOARD_PROXY_API_KEY}"
      },
      "models": {
        "glm-5.1": {
          "name": "GLM-5.1",
          "reasoning": true,
          "limit": {
            "context": 202752,
            "output": 32768
          }
        }
      }
    }
  },
  "model": "switchboard/glm-5.1"
}
```

Then export the proxy key before starting opencode:

```bash
export SWITCHBOARD_PROXY_API_KEY="$PROXY_API_KEY"
```

Verify:

```bash
opencode models switchboard
opencode run --model switchboard/glm-5.1 "Reply with exactly: switchboard ok"
```

Restart opencode after changing config; running sessions keep the old config.

## Pi Coding Agent

Create or edit `~/.pi/agent/models.json`:

```json
{
  "providers": {
    "switchboard": {
      "name": "Switchboard",
      "baseUrl": "http://127.0.0.1:8080/v1",
      "api": "openai-completions",
      "apiKey": "<SWITCHBOARD_PROXY_API_KEY>",
      "compat": {
        "maxTokensField": "max_tokens"
      },
      "models": [
        {
          "id": "glm-5.1",
          "name": "GLM-5.1",
          "reasoning": true,
          "input": ["text"],
          "contextWindow": 202752,
          "maxTokens": 32768
        }
      ]
    }
  }
}
```

Set defaults in `~/.pi/agent/settings.json`, preserving any existing settings
keys:

```json
{
  "defaultProvider": "switchboard",
  "defaultModel": "glm-5.1"
}
```

Verify:

```bash
pi --no-extensions --provider switchboard --model glm-5.1 --no-session --no-tools -p "Reply with exactly: switchboard ok"
```

Expected:

```text
switchboard ok
```
