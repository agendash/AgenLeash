# Installation And Deployment

## Environment

Required for non-local deployments:

| Variable | Description |
| --- | --- |
| `AGENLEASH_TOKEN` | API token. Required unless `AGENLEASH_ALLOW_NO_TOKEN=true`. |
| `AGENLEASH_ADDR` | Listen address. Defaults to `0.0.0.0:8081`. |
| `AGENLEASH_DATA_DIR` | SQLite and service state directory. |
| `AGENLEASH_ADAPTER_DIR` | Adapter spec directory. |
| `AGENLEASH_ALLOWED_WORKSPACE_ROOTS` | Comma, newline, or path-list separated trusted workspace roots. |

History discovery and local CLI state:

| Variable | Description |
| --- | --- |
| `AGENLEASH_DISCOVER_CLAUDE` | Enable Claude Code history discovery. |
| `AGENLEASH_DISCOVER_CODEX` | Enable Codex history discovery. |
| `AGENLEASH_DISCOVER_OPENCODE` | Enable OpenCode history discovery. |
| `AGENLEASH_CLAUDE_HOME` | Claude Code history directory. |
| `AGENLEASH_CODEX_HOME` | Codex history directory. |
| `AGENLEASH_OPENCODE_HOME` | OpenCode history directory. |
| `AGENLEASH_CURSOR_HOME` | Cursor CLI state directory for Docker/local adapter runtimes. |
| `AGENLEASH_GEMINI_HOME` | Gemini CLI state directory for Docker/local adapter runtimes. |
| `AGENLEASH_GROK_HOME` | Grok CLI state directory for Docker/local adapter runtimes. |
| `AGENLEASH_PI_HOME` | Pi CLI state directory for Docker/local adapter runtimes. |
| `AGENLEASH_ACPX_HOME` | ACPX state directory for Docker/local adapter runtimes. |

Optional:

| Variable | Description |
| --- | --- |
| `AGENLEASH_ENABLE_WEB` | Enables `/stats`. Disabled by default. |
| `AGENLEASH_HISTORY_REFRESH_INTERVAL` | Background history refresh interval. |
| `AGENLEASH_SESSION_PERSIST_INTERVAL` | Managed session persistence interval. |
| `AGENLEASH_ALLOW_NO_TOKEN` | Development-only authentication bypass. |

## From Source

```bash
cp .env.example .env
uuidgen | awk '{print "AGENLEASH_TOKEN="$0}' >> .env
AGENLEASH_ENV_FILE=.env go run ./cmd/agenleash
```

## Release Installer

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

Common options:

```bash
AGENLEASH_VERSION=v0.1.0 \
AGENLEASH_PREFIX=/usr/local \
AGENLEASH_INSTALL_SERVICE=systemd \
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

Provide a custom token on first install:

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | \
  env AGENLEASH_TOKEN=my-custom-token sh
```

## Docker Compose

```bash
cp .env.example .env
docker compose up -d --build
```

Edit `.env` before running in production:

```dotenv
AGENLEASH_TOKEN=<strong-random-token>
AGENLEASH_PORT=8081
AGENLEASH_CLAUDE_HOST_DIR=/absolute/path/to/.claude
AGENLEASH_CODEX_HOST_DIR=/absolute/path/to/.codex
AGENLEASH_OPENCODE_HOST_DIR=/absolute/path/to/opencode
AGENLEASH_CURSOR_HOST_DIR=/absolute/path/to/.cursor
AGENLEASH_GEMINI_HOST_DIR=/absolute/path/to/.gemini
AGENLEASH_GROK_HOST_DIR=/absolute/path/to/.grok
AGENLEASH_PI_HOST_DIR=/absolute/path/to/.pi
AGENLEASH_ACPX_HOST_DIR=/absolute/path/to/.acpx
AGENLEASH_XDG_CONFIG_HOST_DIR=/absolute/path/to/.config
AGENLEASH_XDG_CACHE_HOST_DIR=/absolute/path/to/.cache
AGENLEASH_XDG_STATE_HOST_DIR=/absolute/path/to/.local/state
AGENLEASH_LOCAL_BIN_HOST_DIR=/absolute/path/to/.local/bin
AGENLEASH_WORKSPACE_HOST_DIR=/absolute/path/to/workspaces
AGENLEASH_AGENT_BIN_HOST_DIR=/absolute/path/to/agent-bin
```

The Compose stack maps host directories into stable container paths:

| Host purpose | Container path |
| --- | --- |
| Claude Code state | `/home/agenleash/.claude` |
| Codex state | `/home/agenleash/.codex` |
| OpenCode state | `/home/agenleash/.local/share/opencode` |
| Cursor state | `/home/agenleash/.cursor` |
| Gemini state | `/home/agenleash/.gemini` |
| Grok state | `/home/agenleash/.grok` |
| Pi state | `/home/agenleash/.pi` |
| ACPX state | `/home/agenleash/.acpx` |
| XDG config/cache/state | `/home/agenleash/.config`, `/home/agenleash/.cache`, `/home/agenleash/.local/state` |
| User-local binaries | `/home/agenleash/.local/bin` |
| Workspaces | `/workspaces` |
| Optional agent binaries | `/opt/agent-bin` |
| AgenLeash data | `/var/lib/agenleash` |

If the host port `8081` is already in use, set `AGENLEASH_PORT=28081` or another free port. The container can keep `AGENLEASH_ADDR=0.0.0.0:8081`.

## systemd

Template: [packaging/systemd/agenleash.service](../packaging/systemd/agenleash.service)

```bash
sudo install -d /etc/agenleash
sudo cp .env.example /etc/agenleash/agenleash.env
sudo install -m 0644 packaging/systemd/agenleash.service /etc/systemd/system/agenleash.service
sudo systemctl daemon-reload
sudo systemctl enable --now agenleash
```

Adjust `User=`, `Group=`, `Environment=`, and `WorkingDirectory=` before production use.

## launchd

Template: [packaging/launchd/io.agenleash.plist](../packaging/launchd/io.agenleash.plist)

```bash
mkdir -p ~/Library/LaunchAgents
cp packaging/launchd/io.agenleash.plist ~/Library/LaunchAgents/io.agenleash.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/io.agenleash.plist
launchctl enable gui/$(id -u)/io.agenleash
```

To make a background service inherit the CLI search path:

```bash
AGENLEASH_SERVICE_PATH="$PATH" \
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

## Homebrew

Formula template: [packaging/homebrew/agenleash.rb](../packaging/homebrew/agenleash.rb)

Release flow:

1. Build release archives with `VERSION=v0.1.0 ./scripts/build-dist.sh`.
2. Upload archives to a GitHub Release.
3. Update the formula `url` and `sha256`.
4. Push the formula to the tap.
