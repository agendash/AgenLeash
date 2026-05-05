# AgenLeash Adapter Schema

Adapter specs describe how AgenLeash connects to a coding agent family and how behavior changes across agent versions.

The goal is not to hard-code one command. The goal is to model:

- the agent family
- version detection
- launch behavior
- runtime requirements
- workspace policy
- native conversation binding
- history discovery
- capabilities and user-visible features
- event parsing strategy

## Version Boundaries

Keep these version concepts separate:

- `schemaVersion`: adapter file format version
- `adapterRevision`: revision of a specific adapter spec
- `agentVersion`: version of the external coding agent

Do not create separate adapter names for every agent release. Prefer one family-level adapter plus `versionProfiles`.

## Capabilities Versus Features

Capabilities describe whether AgenLeash can launch, host, recover, or control an agent:

- `requiresTTY`
- `requiresRuntimeResize`
- `supportsResume`
- `supportsInterrupt`
- `supportsRawDebug`
- `supportsWorkspaceSwitch`
- `supportsNativeConversationId`

Features describe user-visible behavior exposed by a specific agent version:

- `streamingText`
- `toolCallEvents`
- `structuredPatch`
- `planMode`
- `imageInput`
- `artifactOutput`

Use [FEATURE_REGISTRY.md](FEATURE_REGISTRY.md) for standard feature keys.

## Event Parsing

Structured JSON events are the preferred parser target. ANSI and terminal-control parsing should be treated as compatibility fallback for agents that do not expose structured output yet.

## History Discovery

History discovery should build a metadata index, not a transcript archive. Suggested fields:

- `nativeConversationId`
- `nativeSessionId`
- `sessionFilePath`
- `sourceLocator`
- `workspacePath`
- `workspaceRoot`
- `gitBranch`
- `agentVersion`

Avoid storing transcript bodies, generated summaries, or derived message counts in the AgenLeash metadata cache.

## Example

```yaml
apiVersion: agenleash/v1alpha1
kind: AdapterSpec
metadata:
  name: claudecode
  displayName: Claude Code
  schemaVersion: 1
  adapterRevision: 3
spec:
  agentFamily: claudecode
  detection:
    binaryNames:
      - claude
    versionStrategy:
      type: command
      command: ["claude", "--version"]
      regex: "(?P<version>\\d+\\.\\d+\\.\\d+)"
  runtime:
    mode: stdio
    entrypoint: claude
    args: []
    env: {}
  cwdPolicy:
    mode: required
  capabilities:
    requiresTTY: false
    supportsResume: false
    supportsInterrupt: true
  features:
    streamingText: true
    toolCallEvents: false
  conversation:
    mode: process_only
  workspace:
    mode: cwd_plus_git
  eventParser:
    type: json_events
    profile: default
  versionProfiles:
    - name: legacy
      match: "<1.10.0"
      overrides:
        eventParser:
          type: ansi_rules
          profile: legacy_compat
    - name: stable
      match: ">=1.10.0 <2.0.0"
      overrides:
        capabilities:
          supportsResume: true
          supportsNativeConversationId: true
        features:
          toolCallEvents: true
          structuredPatch: true
        conversation:
          mode: native_id
```
