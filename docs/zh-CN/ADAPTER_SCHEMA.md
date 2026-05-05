# AgenLeash Adapter Schema 设计说明

## 1. 目标

Adapter Schema 用于描述 AgenLeash 如何接入一个 Code Agent 家族，并根据不同版本选择正确的运行时、能力矩阵、会话绑定策略和工作区策略。

这个 Schema 的核心目标不是“描述一个固定命令”，而是描述：

- 这是哪个 **agent family**
- 这个 family 的不同 **version range** 有哪些行为差异
- 这个 family 的不同 **version range** 有哪些 feature 差异
- AgenLeash 如何识别版本
- AgenLeash 如何在不同版本下选择正确的 `capabilities`、`features`、`conversation` 和 `workspace` 策略

一句话说：**Adapter 是 family 级抽象，Version Profile 是具体行为档位。**

---

## 2. 设计原则

### 2.1 区分三种版本

Schema 中至少要区分以下三类版本：

- `schema_version`：Adapter 配置格式本身的版本
- `adapter_revision`：某份 Adapter 配置内容的修订号
- `agent_version`：被接入的 Code Agent 实际版本

这三者不能混用。

### 2.2 按 family 建模，不按单版本建模

不要为每个 agent 版本单独创建一个 Adapter 名称，例如：

- 不推荐：`claudecode-v1`, `claudecode-v2`, `claudecode-v3`
- 推荐：`claudecode` + `version_profiles`

这样可以避免启动入口和权限配置碎片化，也更适合持续兼容。

### 2.3 能力默认保守

当版本无法识别时，必须回退到保守档位：

- 不默认开启 `supports_resume`
- 不默认开启 `supports_interrupt`
- 不默认假设存在稳定的 `native_conversation_id`
- 不默认假设输出格式稳定

### 2.4 能力可覆盖，不可硬编码

以下字段都应允许按版本覆盖：

- `runtime_mode`
- `capabilities`
- `features`
- `conversation`
- `workspace`
- `event_parser`
- `hooks`

### 2.5 区分 capabilities 和 features

Schema 中建议显式区分两类信息：

- `capabilities`：AgenLeash 接入层和运行时能力
- `features`：某个 agent/version 实际对外提供的功能特性

例如：

- `requiresTTY` 属于 capability
- `supportsResume` 更偏接入能力，也建议放在 capability
- `plan_mode` 属于 feature
- `structured_patch` 属于 feature
- `tool_call_events` 属于 feature
- `image_input` 属于 feature

这两个集合都可以按版本变化，但语义不同，不能混用。

标准 `features` key 应优先复用 [FEATURE_REGISTRY.md](./FEATURE_REGISTRY.md) 中的定义；只有在 registry 尚未覆盖时，才允许使用 namespaced 自定义 key。

### 2.6 事件解析以 JSON 优先

AgenLeash 的适配器事件解析应以结构化 JSON 事件为主，ANSI / 光标 / 终端控制序列只作为 legacy fallback。

也就是说：

- `json_events` 应作为默认的主事件解析路径
- `ansi_rules` 只适用于尚未提供结构化事件的旧版或兼容型 Adapter
- 任何能直接输出 JSON event 的 agent/version，都不应再被建模成 ANSI-first

### 2.7 历史会话发现只做 Meta Index

Adapter 如果需要声明本机历史 conversation/session 的发现逻辑，目标应是帮助 AgenLeash 建立一个稳定的 **session meta index**，而不是复制原始对话内容。

建议历史索引只产出能够定位原始数据的字段，例如：

- `nativeConversationId` 或 `nativeSessionId`
- `sessionFilePath`
- `sourceLocator`
- `workspacePath`
- `workspaceRoot`
- `gitBranch`
- `agentVersion`（仅当源数据天然提供）

不建议通过 AgenLeash 的 SQLite meta index cache 持久化以下内容：

- transcript 正文
- 自动生成的 `summary`
- `message_count`
- 任意逐条消息副本

---

## 3. 顶层结构

建议每个 Adapter 文件采用如下结构：

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
      - claudecode
    versionStrategy:
      type: command
      command: ["claudecode", "--version"]
      regex: "(?P<version>\\d+\\.\\d+\\.\\d+)"
  runtime:
    mode: pty
    entrypoint: claudecode
    args: []
    env: {}
  cwdPolicy:
    mode: required
  capabilities:
    requiresTTY: true
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
        capabilities:
          supportsResume: false
        features:
          toolCallEvents: false
        conversation:
          mode: process_only
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
          extractors:
            - type: regex
              source: stdout
              pattern: "Conversation ID: (?P<id>[A-Za-z0-9_-]+)"
```

---

## 4. 字段设计

### 4.1 `metadata`

建议字段：

- `name`：Adapter 名称，系统内唯一
- `displayName`：展示名称
- `schemaVersion`：当前配置格式版本
- `adapterRevision`：该配置内容的修订号

### 4.2 `spec.agentFamily`

用于标识被接入 agent 的家族名，例如：

- `claudecode`
- `aider`
- `cursor-cli`

`agentFamily` 是版本识别和能力矩阵匹配的根键。

### 4.3 `spec.detection`

用于识别当前宿主机上被启动的 agent 版本。

建议字段：

- `binaryNames`
- `versionStrategy`
- `detectVersion`
- `fallbackProfile`

#### `versionStrategy.type` 建议取值

- `command`：执行 `--version` 类命令
- `regex_stdout`：从启动输出中提取
- `file_probe`：从元信息文件读取
- `custom_hook`：调用自定义检测钩子

### 4.4 `spec.runtime`

定义基础运行时。

建议字段：

- `mode`：`pty` / `stdio` / `native`
- `entrypoint`
- `args`
- `env`
- `startupTimeoutSec`

设计建议：

- 若 agent/version 支持 `stdio/non-interactive`，Adapter 应优先声明为 `stdio`
- `pty` 应视为兼容模式，只在该版本明确依赖真实 TTY 时才启用
- 是否需要 `resize`、是否接受 `\r` 作为提交键，应由 `capabilities.requiresTTY` 和相关 capability 联合表达，而不是让前端猜测

### 4.5 `spec.cwdPolicy`

定义工作目录要求。

建议字段：

- `mode`：`required` / `optional` / `forbidden`
- `allowedRoots`
- `requireGitRoot`
- `resolveSymlink`

### 4.6 `spec.capabilities`

定义基础能力。这里应理解为 **baseline**，后续可以被版本档位覆盖。

建议字段：

- `requiresTTY`
- `requiresRuntimeResize`
- `supportsResume`
- `supportsInterrupt`
- `supportsRawDebug`
- `supportsNativeConversationId`
- `supportsWorkspaceSwitch`
- `supportsStructuredOutput`

### 4.7 `spec.features`

定义 agent/version 对外暴露的功能特性。这里同样应理解为 **baseline**，并允许被版本档位覆盖。

建议字段：

- `streamingText`
- `toolCallEvents`
- `structuredPatch`
- `planMode`
- `imageInput`
- `artifactOutput`
- `multiWorkspace`

这些字段主要用于：

- 告诉 AgenDash 某个 UI 功能是否应该显示
- 告诉 AgenLeash 某类事件是否应该尝试解析
- 在版本升级后做 feature gate

字段命名和语义边界应以 [FEATURE_REGISTRY.md](./FEATURE_REGISTRY.md) 为准。

### 4.8 `spec.conversation`

定义会话绑定和恢复策略。

建议字段：

- `mode`：`process_only` / `native_id` / `external_api`
- `resumeStrategy`
- `extractors`
- `historySources`
- `sessionLocator`
- `bindTimeoutSec`
- `allowResumeAcrossRestart`

#### `extractors` 建议

支持多条提取规则，按顺序尝试：

- 从 `stdout` 正则提取
- 从日志文件提取
- 从环境输出提取
- 从状态文件提取

#### `historySources` 建议

用于声明如何从本机历史目录或数据库发现历史 conversation/session 的索引入口。

建议支持的 source 类型包括：

- `file_index`
- `sqlite`
- `jsonl`
- `custom_hook`

#### `sessionLocator` 建议

用于声明 discovered conversation/session 如何回指原始存储位置。

建议字段：

- `sessionFilePath`
- `sourceLocator`
- `idField`

这里的目标是让 AgenLeash 能缓存“这个 session 在哪里”，而不是缓存“这个 session 说了什么”。

### 4.9 `spec.workspace`

定义工作区识别和校验逻辑。

建议字段：

- `mode`：`cwd_only` / `cwd_plus_git` / `custom`
- `detectGit`
- `fingerprint`
- `allowWorkspaceSwitch`
- `metadataFiles`

#### `fingerprint` 建议字段

- `cwd`
- `git_root`
- `git_remote`
- `custom_project_id`

### 4.10 `spec.eventParser`

定义如何把原始输出归一化为 AgenLeash 事件。

建议字段：

- `type`
- `profile`
- `rules`
- `flushPolicy`

#### `type` 建议取值

- `json_events`
- `ansi_rules`
- `line_rules`
- `custom_hook`

### 4.11 `spec.versionProfiles`

这是 Schema 的关键部分，用于声明同一 agent family 在不同版本范围下的差异。

每个 profile 建议包括：

- `name`
- `match`
- `priority`
- `overrides`

其中 `match` 推荐采用 SemVer range 表达式，例如：

- `<1.10.0`
- `>=1.10.0 <2.0.0`
- `>=2.0.0`

---

## 5. 版本匹配与合并规则

### 5.1 匹配流程

服务端应按以下顺序计算有效配置：

1. 载入 family 级基础配置
2. 识别实际 `agent_version`
3. 在 `versionProfiles` 中选出匹配项
4. 按优先级合并 `overrides`
5. 生成最终 `effective_spec`

### 5.2 优先级建议

建议优先级从低到高为：

1. family 基础配置
2. 匹配到的 version profile
3. 启动请求中的显式 hint
4. 服务端动态探测结果

### 5.3 合并策略

建议采用以下规则：

- 标量字段：后者覆盖前者
- 对象字段：递归合并
- 数组字段：默认整体替换，不做隐式拼接

数组不建议自动拼接，否则不同版本的解析规则容易相互污染。

### 5.4 未知版本处理

当无法识别版本，或没有任何 profile 匹配时：

- 使用 `fallbackProfile`，若存在
- 否则使用 family 基础配置
- 同时将 Session 标记为 `version_unverified`

---

## 6. 会话与工作区相关字段的版本差异处理

版本差异最容易影响的不是启动命令，而是 Conversation 和 Workspace 语义。

### 6.1 Conversation 差异

不同版本可能出现以下变化：

- 新版本才支持稳定的 `conversation_id`
- 老版本只能依赖进程存活恢复
- 不同版本提取 `conversation_id` 的正则模式不同
- 某些版本恢复命令行参数不同

因此，这些字段必须支持 version override：

- `conversation.mode`
- `conversation.resumeStrategy`
- `conversation.extractors`
- `conversation.historySources`
- `conversation.sessionLocator`
- `capabilities.supportsResume`
- `capabilities.supportsNativeConversationId`

### 6.2 Workspace 差异

不同版本可能出现以下变化：

- 工作区从单纯 `cwd` 升级为 `cwd + git`
- 新版本允许切换工作区，旧版本不允许
- 新版本会生成 metadata 文件，旧版本没有
- Git 分支探测逻辑或 project root 逻辑变化

因此，这些字段必须支持 version override：

- `workspace.mode`
- `workspace.fingerprint`
- `workspace.allowWorkspaceSwitch`
- `capabilities.supportsWorkspaceSwitch`

---

## 7. 推荐的实现数据模型

在服务端内存或数据库中，建议把以下对象分开存储：

- `AdapterSpec`
- `ResolvedAdapterProfile`
- `SessionRuntimeBinding`

### 7.1 `AdapterSpec`

表示 family 级静态配置。

### 7.2 `ResolvedAdapterProfile`

表示某次启动后，根据 `agent_version` 解析出的最终有效配置。

建议字段：

- `adapter_name`
- `agent_family`
- `resolved_version`
- `matched_profile`
- `effective_runtime_mode`
- `effective_capabilities`
- `effective_features`
- `effective_conversation_mode`
- `effective_workspace_mode`
- `effective_parser`

### 7.3 `SessionRuntimeBinding`

表示某个 Session 实际绑定到的版本档位和运行时。

建议字段：

- `session_id`
- `adapter_name`
- `resolved_version`
- `matched_profile`
- `runtime_pid`
- `native_conversation_id`
- `workspace_fingerprint`

---

## 8. `claudecode` 示例

下面给出一个更贴近实际的示意例子，用来说明如何区分不同版本能力。

```yaml
apiVersion: agenleash/v1alpha1
kind: AdapterSpec
metadata:
  name: claudecode
  displayName: Claude Code
  schemaVersion: 1
  adapterRevision: 1

spec:
  agentFamily: claudecode

  detection:
    binaryNames: ["claudecode"]
    versionStrategy:
      type: command
      command: ["claudecode", "--version"]
      regex: "(?P<version>\\d+\\.\\d+\\.\\d+)"
    fallbackProfile: unknown

  runtime:
    mode: pty
    entrypoint: claudecode
    args: []
    env:
      TERM: xterm-256color
    startupTimeoutSec: 20

  cwdPolicy:
    mode: required
    requireGitRoot: false
    resolveSymlink: true

  capabilities:
    requiresTTY: true
    requiresRuntimeResize: false
    supportsResume: false
    supportsInterrupt: true
    supportsRawDebug: true
    supportsNativeConversationId: false
    supportsWorkspaceSwitch: false
    supportsStructuredOutput: false

  features:
    streamingText: true
    toolCallEvents: false
    structuredPatch: false
    planMode: false
    imageInput: false
    artifactOutput: false
    multiWorkspace: false

  conversation:
    mode: process_only
    resumeStrategy: process_only
    bindTimeoutSec: 10
    allowResumeAcrossRestart: false
    extractors: []

  workspace:
    mode: cwd_plus_git
    detectGit: true
    allowWorkspaceSwitch: false
    fingerprint:
      - cwd
      - git_root
      - git_remote

  eventParser:
    type: ansi_rules
    profile: baseline

  versionProfiles:
    - name: unknown
      match: "*"
      priority: 0
      overrides:
        capabilities:
          supportsResume: false
          supportsNativeConversationId: false
        features:
          toolCallEvents: false
          structuredPatch: false
        conversation:
          mode: process_only
          resumeStrategy: process_only
        eventParser:
          type: json_events
          profile: unknown

    - name: v1-legacy
      match: "<1.10.0"
      priority: 10
      overrides:
        eventParser:
          type: ansi_rules
          profile: legacy

    - name: v1-stable
      match: ">=1.10.0 <2.0.0"
      priority: 20
      overrides:
        capabilities:
          supportsResume: true
          supportsNativeConversationId: true
        features:
          toolCallEvents: true
          structuredPatch: true
        conversation:
          mode: native_id
          resumeStrategy: native_id+workspace
          extractors:
            - type: regex
              source: stdout
              pattern: "Conversation ID: (?P<id>[A-Za-z0-9_-]+)"
        eventParser:
          type: json_events
          profile: stable_v1

    - name: v2
      match: ">=2.0.0"
      priority: 30
      overrides:
        capabilities:
          supportsResume: true
          supportsNativeConversationId: true
          supportsStructuredOutput: true
        features:
          toolCallEvents: true
          structuredPatch: true
          artifactOutput: true
        conversation:
          mode: native_id
          resumeStrategy: native_id+workspace
          extractors:
            - type: file
              path: ".claudecode/session.json"
              jsonPath: "$.conversation.id"
        workspace:
          mode: custom
          metadataFiles:
            - ".claudecode/session.json"
        eventParser:
          type: json_events
          profile: v2_structured
```

这个例子说明：

- 同一个 adapter 名称可以覆盖多个 agent 版本
- 是否支持恢复，不是 family 常量，而是版本档位能力
- 是否支持某个 feature，也不是 family 常量，而是版本档位特性
- `conversation` 的提取方式可以因版本不同而变化
- `workspace` 的识别逻辑也可以因版本不同而变化

---

## 9. 建议落地方式

建议第一版实现时采用以下分层：

1. 一个 family 一个 Adapter 文件
2. 文件内部通过 `versionProfiles` 管理版本差异
3. 启动时解析成 `ResolvedAdapterProfile`
4. Session 只持久化最终解析结果，不反复回看原始配置

这样可以同时兼顾：

- 配置可读性
- 版本兼容性
- 故障排查可追溯性
- 后续多 agent 扩展能力
