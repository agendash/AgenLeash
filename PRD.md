# AgenLeash Product Requirements

## Positioning

AgenLeash is a lightweight runtime and session gateway for coding agents. It runs on a host machine, starts and supervises local agent processes, and exposes their sessions through a stable HTTP and WebSocket API for AgenDash and compatible clients.

AgenLeash is not an inference service, planner, or task orchestrator. It does not implement ASR, LLM, or TTS capability. Voice flows should go through AgenSense first, then enter AgenDash or AgenLeash as normal session input.

Recommended local development ports:

- AgenDash: application dependent
- AgenSense: `127.0.0.1:8080`
- AgenLeash: `127.0.0.1:8081`

## Goals

- Start, supervise, interrupt, and recover coding agent sessions safely.
- Provide a single session model across Codex, Claude Code, OpenCode, and future adapters.
- Normalize agent output into structured events for chat-style GUI clients.
- Discover local agent history metadata without copying full transcripts.
- Preserve mappings between sessions, native conversations, workspaces, and source locators.
- Support reconnectable WebSocket streams and durable session metadata.
- Require token authentication by default.

## Non-Goals

- AgenLeash does not provide model inference.
- AgenLeash does not render the AgenDash UI.
- AgenLeash does not make terminal emulation the primary client protocol.
- AgenLeash does not store full third-party agent transcripts in its metadata cache.
- AgenLeash does not provide a full multi-tenant scheduler in the MVP.

## Core Concepts

### Runtime Compatibility

Adapters choose how an agent is launched:

- `stdio`: standard input and output for non-interactive agent modes.
- `pty`: pseudo-terminal mode for agents that require a real TTY.
- `native`: future direct API or event-stream integrations.

STDIO should be preferred when an agent supports it. PTY is an internal compatibility layer, not the public protocol.

### Session Event Protocol

The public event surface is JSON-first:

- `session_snapshot`
- `message_started`
- `message_delta`
- `message_completed`
- `input_requested`
- `state_changed`
- `conversation_bound`
- `workspace_updated`
- `sync_end`
- `raw_chunk` for debug-only compatibility

Clients should not infer session meaning from ANSI cursor movement or terminal escape sequences.

### Adapter Layer

Each adapter describes:

- agent family and version detection
- launch command and runtime mode
- environment variables
- workspace policy
- capabilities and features
- native conversation binding
- history discovery
- event parsing

Adapter specs are family-level descriptions. Version-specific behavior belongs in `versionProfiles`, not separate one-off adapter names.

### History Metadata Index

AgenLeash discovers local agent history so AgenDash can list existing conversations. The metadata cache should store only enough information to locate the original session:

- adapter
- native conversation ID
- workspace path
- workspace root
- git branch
- source locator or detail path

It should not copy full transcripts, summaries, or message counts unless a future product requirement explicitly needs that data.

## Security Model

- `AGENLEASH_TOKEN` is required by default.
- `AGENLEASH_ALLOW_NO_TOKEN=true` is local development only.
- Managed sessions should be restricted with `AGENLEASH_ALLOWED_WORKSPACE_ROOTS`.
- File preview APIs must stay inside discovered or allowed workspaces.
- Dashboard access requires the same token as the API.

## Success Criteria

- AgenDash can start and reconnect to coding agent sessions through a stable API.
- Local agent history is visible as discovered sessions without duplicating transcripts.
- Adapter capability and feature data is consistent enough for UI feature gates.
- Release artifacts exclude local state, credentials, logs, and generated runtime data.
