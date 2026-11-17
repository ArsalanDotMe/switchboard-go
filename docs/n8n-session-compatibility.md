# Stock n8n Assistant session compatibility

Switchboard accepts n8n Assistant's native request labels and maps the actual
thread identifier to the session header required by OpenCode Go. No n8n source
patch, message history storage, or conversation-guessing heuristic is needed.

## Setup

In the n8n environment, enable its supported model request labels:

```yaml
N8N_INSTANCE_AI_MODEL_REQUEST_LABELS: "true"
```

Restart n8n using your normal deployment procedure. Configure the Assistant's
custom OpenAI-compatible provider with your Switchboard `/v1` base URL, existing
proxy API key, and an available model. Keep TLS and gateway authentication on.
Ensure your private reverse proxy permits the actual n8n source address. A 403
before gateway authentication is an ingress problem, not necessarily a bad key.
Do not trust arbitrary forwarded headers or broadly allow Docker networks.

The installed contract was tested with stock n8n 2.47.7. Future n8n changes to
request-label or verification-probe formats require revalidation.

## Mapping and limits

For authenticated POST chat-completion requests, when no session is supplied:

- `x-n8n-feature` must be `instance-ai`.
- `x-n8n-thread-id` and `x-n8n-version` must be nonempty bounded tokens using
  letters, digits, periods, underscores, colons, or hyphens.
- The session becomes `n8n-<thread-id>`, stable across turns and isolated across
  different thread IDs. Configure unique thread IDs across instances sharing
  an upstream account.
- The User-Agent truthfully identifies Switchboard and n8n Assistant. The
  compatibility implementation's gateway label is `switchboard-go/1.4.0`.

An existing nonempty `x-opencode-session` and its User-Agent are preserved
verbatim. Other clients remain responsible for their own session metadata.
Ordinary unlabeled inference is not assigned a global session or inferred from
IP address, API key, model, prompt hash, or shared message prefixes.

Stock n8n's model verification omits thread labels. Its exact standalone probe
is recognized only with an AI SDK User-Agent, no thread label, exactly the
`model`, `max_tokens: 16`, and `messages` fields, and one user message containing
`Reply with OK.`. It receives a unique cryptographic verification session and
truthful verification identity. Additional fields, tools, or other prompts do
not qualify. This bounded connectivity probe is not a conversation fallback.
The session is generated once before upstream key retries.

Authorization replacement, bodies, routing, streaming, and supplied session
headers retain existing behavior. This mapping is not an authorization boundary:
request labels are caller-provided metadata, processed after gateway API-key
authentication.

## Verification

```sh
go test -race -v ./...
go vet ./...
go build ./...
```

Regression tests observe upstream headers for stable A/B/A sessions, unique
verification IDs, existing-client preservation, invalid/missing labels, no
ambiguous fallback, unchanged payload/authentication, and SSE passthrough.
Live transport validation used stock n8n's `verifyModel` and two concurrent,
two-turn synthetic conversations. Provider transport success does not prove
full Assistant sandbox/tool readiness, or that the owner saved the model dialog.
Do not use private workflows or rerun side-effecting workflows as connectivity
tests.

## Upstream contract

Use the upstream only within its supported third-party-client terms, with
truthful identity and real per-conversation sessions. This integration does not
impersonate another client or bypass upstream authorization.

- [n8n Assistant setup](https://docs.n8n.io/deploy/host-n8n/configure-n8n/set-up-n8n-assistant)
- [OpenCode Go supported clients](https://opencode.ai/docs/console/go/#where-can-i-use-it)
