# AgenLeash Feature Registry

This registry defines standard `features` keys for adapter specs and session metadata.

It keeps UI feature gates consistent across agent families and agent versions.

## Feature Definition

A feature is a user-visible behavior that a specific agent version can provide in a stable enough way for clients to react to it.

Examples:

- `streamingText`
- `toolCallEvents`
- `structuredPatch`
- `planMode`
- `imageInput`
- `artifactOutput`

A feature is not:

- a host runtime requirement
- a temporary health state
- a one-session success or failure
- an internal parser implementation detail

## Capability Boundary

Use adapter capabilities for runtime-level behavior:

- `requiresTTY`
- `requiresRuntimeResize`
- `supportsResume`
- `supportsInterrupt`
- `supportsNativeConversationId`

Use features for client-visible behavior:

- `planMode`
- `toolCallEvents`
- `imageInput`
- `artifactOutput`

## Naming

Standard keys use `lowerCamelCase`.

Prefer reusable semantic names:

- `toolCallEvents`
- `structuredPatch`
- `planMode`

Avoid agent-specific or implementation-specific names:

- `has_plan`
- `tool_events_v2`
- `claudecodeToolCalls`

Experimental keys should be namespaced:

```text
x.<agentFamily>.<featureName>
```

Examples:

- `x.claudecode.reviewLoop`
- `x.aider.repoMapHints`

## Standard Keys

| Key | Meaning |
| --- | --- |
| `streamingText` | Agent can stream text deltas. |
| `toolCallEvents` | Agent exposes structured tool call lifecycle events. |
| `structuredPatch` | Agent can emit structured code patch data. |
| `planMode` | Agent supports an explicit planning mode. |
| `imageInput` | Agent can accept image input. |
| `artifactOutput` | Agent can produce explicit artifacts or generated files. |
| `multiWorkspace` | Agent can reason across more than one workspace. |
| `reviewWorkflow` | Agent supports a review-oriented workflow. |

## Session Metadata

After adapter resolution, sessions should expose normalized `effective_features`:

```json
{
  "effective_features": {
    "streamingText": true,
    "toolCallEvents": true,
    "structuredPatch": false,
    "planMode": true
  }
}
```

Clients should read `effective_features`, not raw adapter files.
