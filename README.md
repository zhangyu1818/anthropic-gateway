# anthropic-gateway

A configuration-driven Anthropic-compatible gateway written in Go.

## Features

- Exposes:
  - `POST /anthropic/v1/messages`
  - `POST /anthropic/v1/messages/count_tokens`
  - `GET /anthropic/v1/models`
  - `GET /healthz`
- Rewrites `model`, `api_base`, and upstream auth from YAML.
- Supports SSE streaming passthrough (`stream: true`).
- Returns Anthropic-style error JSON.
- Does not validate inbound auth tokens; it always replaces auth for upstream.

## Quick Start

1. Copy and edit config:

```bash
cp config.example.yaml config.yaml
```

2. Set env vars used in config:

```bash
export UPSTREAM_API_KEY=your-key
```

3. Run:

```bash
go run ./cmd/anthropic-gateway -c config.yaml
```

Or build a binary:

```bash
go build -o anthropic-gateway ./cmd/anthropic-gateway
./anthropic-gateway -c config.yaml
```

## Autostart (macOS)

Install launch agent (requires config path):

```bash
./anthropic-gateway autostart install -c /absolute/path/to/config.yaml
```

Check status:

```bash
./anthropic-gateway autostart status
```

Remove autostart:

```bash
./anthropic-gateway autostart uninstall
```

## Config

```yaml
listen: ":4000" # optional, default :4000

model_list:
  - model_name: opus
    params:
      model: glm-5
      api_base: https://your-upstream.example.com
      api_key: ${UPSTREAM_API_KEY}
      auth_type: x-api-key # x-api-key | bearer

  - model_name: sonnet
    params:
      model: glm-5
      api_base: https://your-upstream.example.com
      api_key: ${UPSTREAM_API_KEY}
      auth_type: x-api-key # x-api-key | bearer

  - model_name: haiku
    params:
      model: glm-4.7
      api_base: https://your-upstream.example.com
      api_key: ${UPSTREAM_API_KEY}
      auth_type: x-api-key # x-api-key | bearer
```

### Route Behavior

- Request `model` must match `model_list[].model_name`.
- Gateway rewrites outbound model to `model_list[].params.model`.
- Gateway targets `model_list[].params.api_base + incoming path suffix`.
- Auth replacement:
  - `auth_type: x-api-key` -> `x-api-key: <api_key>`
  - `auth_type: bearer` -> `Authorization: Bearer <api_key>`

## Error Semantics

- Unknown model / invalid JSON / missing model: `400`
- Unsupported `/anthropic/*` path: `404`
- Upstream connection failure: `502`
- Non-Anthropic upstream error payloads are normalized to Anthropic-style errors.

## Development

```bash
go test ./...
go test -race ./...
```
