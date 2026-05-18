# AgenLeash

AgenLeash is a lightweight Code Agent runtime gateway that starts, supervises, and normalizes local coding agents into secure HTTP and WebSocket sessions for AgenDash and compatible clients.

It does not run AI inference itself. Its job is to turn tools such as Codex, Claude Code, OpenCode, Cursor CLI, Gemini CLI, Grok Build, Pi, ACPX, and other command-line coding agents into a stable service boundary that desktop, web, and voice clients can control.

## What It Does

- Starts and supervises local coding agent processes.
- Exposes a unified HTTP API for session lifecycle, history discovery, and workspace access.
- Streams session events over WebSocket for chat-style GUI clients.
- Discovers local Codex, Claude Code, and OpenCode history metadata.
- Normalizes adapter capabilities, feature flags, workspace roots, and native conversation IDs.
- Stores only service metadata locally; original agent transcripts remain in the agent-owned stores.

## Repository Description

Use this as the GitHub repository description:

```text
Lightweight runtime and session gateway for hosting Codex, Claude Code, OpenCode, and other local coding agents behind a unified HTTP/WebSocket API for AgenDash.
```

## Quick Start

### Run From Source

```bash
cp .env.example .env
uuidgen | awk '{print "AGENLEASH_TOKEN="$0}' >> .env
AGENLEASH_ENV_FILE=.env go run ./cmd/agenleash
```

The server listens on `0.0.0.0:8081` by default.

### Install From Release

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

The installer generates a UUID token on first run unless `AGENLEASH_TOKEN` is already set.

To provide your own token:

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | \
  env AGENLEASH_TOKEN=my-custom-token sh
```

### Run With Docker Compose

```bash
cp .env.example .env
docker compose up -d --build
```

Before exposing the container outside a trusted local machine, set a strong `AGENLEASH_TOKEN` and restrict `AGENLEASH_ALLOWED_WORKSPACE_ROOTS`.

## Configuration

Minimum production configuration:

```dotenv
AGENLEASH_TOKEN=<strong-random-token>
AGENLEASH_ADDR=0.0.0.0:8081
AGENLEASH_DATA_DIR=/var/lib/agenleash
AGENLEASH_ADAPTER_DIR=adapters
AGENLEASH_ALLOWED_WORKSPACE_ROOTS=/workspaces,/srv/repos
```

Common history discovery paths:

```dotenv
AGENLEASH_CLAUDE_HOME=/home/you/.claude
AGENLEASH_CODEX_HOME=/home/you/.codex
AGENLEASH_OPENCODE_HOME=/home/you/.local/share/opencode
```

Docker Compose also mounts local CLI state for Cursor, Gemini, Grok, Pi, and ACPX. Set the matching `AGENLEASH_*_HOST_DIR` values in `.env` before starting the container, and put CLI binaries in either `AGENLEASH_AGENT_BIN_HOST_DIR` or `AGENLEASH_LOCAL_BIN_HOST_DIR`.

The service refuses to start without `AGENLEASH_TOKEN` unless `AGENLEASH_ALLOW_NO_TOKEN=true` is set explicitly. That opt-out is intended for local development only.

## HTTP And WebSocket API

Core endpoints:

- `GET /healthz`
- `GET /api/v1/sessions`
- `POST /api/v1/agent/start`
- `GET /api/v1/sessions/{session_id}`
- `POST /api/v1/sessions/{session_id}/messages`
- `GET /ws/v1/sessions/{session_id}/events`

Authenticated requests use:

```http
X-AgenLeash-Token: <token>
```

The WebSocket endpoint also accepts `?token=<token>` for clients that cannot set headers during the upgrade.

## Adapters

Adapter specs live in [adapters](adapters). They describe how AgenLeash starts an agent family, detects versions, models capabilities, maps workspaces, and parses events.

Built-in adapters:

- `codex` / `codex_local`
- `claudecode` / `claude_local`
- `opencode` / `opencode_local`
- `cursor`
- `gemini_local`
- `grok_local`
- `pi_local`
- `acpx_local`
- `mock-adapter`

See [docs/ADAPTER_SCHEMA.md](docs/ADAPTER_SCHEMA.md) and [docs/FEATURE_REGISTRY.md](docs/FEATURE_REGISTRY.md) for the schema and standard feature keys.

## Dashboard

The browser dashboard is disabled by default.

Enable it with:

```dotenv
AGENLEASH_ENABLE_WEB=true
```

Then open:

```text
http://<host>:<port>/stats
```

The page asks for `AGENLEASH_TOKEN` before loading service stats.

## Development

```bash
go test ./...
go build ./cmd/agenleash
```

Build release artifacts:

```bash
VERSION=v0.1.0 ./scripts/build-dist.sh
```

Runtime output, local SQLite state, release archives, logs, and generated test data are excluded from Git by default.

## Documentation

- [Documentation index](docs/README.md)
- [Installation and deployment](docs/INSTALL.md)
- [Adapter schema](docs/ADAPTER_SCHEMA.md)
- [Feature registry](docs/FEATURE_REGISTRY.md)
- [Release process](docs/RELEASE.md)
- [Chinese documentation](README.zh-CN.md)
