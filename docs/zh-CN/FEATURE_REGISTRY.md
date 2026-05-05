# AgenLeash Standard Feature Registry

## 1. 目标

本文件用于定义 AgenLeash 中 `features` 字段的标准 key、语义边界和使用规则。

它解决三个问题：

- 不同 Adapter 不再各自发明 feature 名称
- 同一个 Code Agent 的不同版本，可以用同一套标准 key 表达 feature 差异
- AgenDash 和 AgenLeash 可以基于同一份 registry 做 feature gate

这个 registry 只描述对外可见的功能特性，不把 ANSI 解析当成主能力；主事件面应以结构化 JSON 为准，terminal 兼容只作为迁移辅助。

一句话说：**Feature Registry 是 AgenLeash 对“agent/version 对外功能特性”的统一词表。**

---

## 2. Feature 是什么

在 AgenLeash 中，`feature` 指的是：

- 某个 Code Agent 的某个版本，对外稳定提供的功能特性
- 会影响前端 UI、事件解析、交互流程或 artifact 呈现
- 可以随 agent 版本变化而变化

Feature 不是：

- 宿主运行时能力
- 当前机器环境健康状态
- 某次会话里临时失败或成功的运行结果

例如：

- `planMode` 是 feature
- `imageInput` 是 feature
- `structuredPatch` 是 feature
- `requiresTTY` 不是 feature，它是 capability
- `supportsResume` 更偏运行接入能力，也不建议放到 feature

---

## 3. 与 Capability 的边界

建议用下面的判定规则：

- 如果它描述“服务端是否能正确启动、接入、托管、恢复某个 agent”，它更像 `capability`
- 如果它描述“这个 agent/version 对 UI 和上层交互暴露了什么功能”，它更像 `feature`

### 3.1 Capability 示例

- `requiresTTY`
- `requiresRuntimeResize`
- `supportsResume`
- `supportsInterrupt`
- `supportsNativeConversationId`

### 3.2 Feature 示例

- `streamingText`
- `toolCallEvents`
- `structuredPatch`
- `planMode`
- `imageInput`
- `artifactOutput`

### 3.3 不要混用

以下写法都不推荐：

- 用 `supportsPlan` 这种 capability 风格字段表达 UI feature
- 用 `resumeSession` 这种 feature 风格字段表达恢复能力
- 把“运行时要求”和“用户可见特性”放进同一个 map

---

## 4. 数据模型

### 4.1 Adapter 配置中的 `features`

建议定义为扁平布尔 map：

```yaml
features:
  streamingText: true
  toolCallEvents: false
  structuredPatch: true
  planMode: false
```

语义如下：

- `true`：当前版本档位下，该 feature 可用且可以对外承诺
- `false`：当前版本档位下，该 feature 不可用，或不应对外承诺
- `省略`：表示继承上层配置；合并完成后的 `effective_features` 不应保留歧义

### 4.2 Session 元数据中的 `effective_features`

Session 启动并完成版本匹配后，服务端应输出归一化后的 `effective_features`。

建议要求：

- 对所有标准 stable feature key 给出显式布尔值
- 对未命中的 experimental/custom feature，按最终合并结果保留
- 前端只根据 `effective_features` 做功能显隐，不直接读原始 adapter 文件

---

## 5. 命名规则

### 5.1 标准 key 命名

- 使用 `lowerCamelCase`
- 名称应表达“对外功能语义”，而不是实现细节
- 名称应尽量跨 agent family 复用

推荐：

- `planMode`
- `toolCallEvents`
- `imageInput`

不推荐：

- `has_plan`
- `tool_events_v2`
- `claudecodeToolCalls`

### 5.2 自定义 feature

如果某个 feature 尚未进入标准 registry，但确实需要提前使用，必须采用 namespaced key。

建议格式：

- `x.<agentFamily>.<featureName>`

例如：

- `x.claudecode.reviewLoop`
- `x.aider.repoMapHints`

规则：

- 自定义 key 只能作为临时扩展
- 一旦被两个以上 Adapter 复用，应提升为标准 key
- 前端默认不应对未知自定义 key 做强依赖

### 5.3 不兼容变更

如果某个现有 feature 的语义发生根本变化，不要复用旧 key。

应新增新 key，并标记旧 key 为 `deprecated`。

---

## 6. 标准 Feature 列表

以下为建议的第一版标准 registry。

### 6.1 消息与输出类

#### `streamingText`

- 含义：Agent 可以持续产出增量文本，而不是只能一次性返回整条消息
- 主要影响：
  - 是否启用 `message_delta`
  - 是否显示打字流式渲染
- 备注：这通常是最基础的聊天特性之一

#### `jsonEventStream`

- 含义：Agent / Adapter 可以稳定输出可直接消费的结构化 JSON 事件流
- 主要影响：
  - 前端和上层服务是否以 JSON 事件为主进行编排
  - 是否把 ANSI 解析降级为兼容路径

#### `fileReferences`

- 含义：Agent 输出中可以稳定附带文件引用信息，且这些引用可被结构化消费
- 主要影响：
  - 前端是否展示可点击文件引用
  - 服务端是否尝试解析文件定位信息

#### `artifactOutput`

- 含义：Agent 可以输出独立于普通消息正文的 artifact，例如报告、补丁包、计划稿、摘要文件
- 主要影响：
  - 前端是否显示 artifact 面板
  - 服务端是否维护 artifact 列表和更新事件

### 6.2 工具与执行类

#### `toolCallEvents`

- 含义：Agent 能以结构化方式暴露工具调用开始、进展和结束
- 主要影响：
  - 前端是否展示工具调用时间线
  - 服务端是否产出 `tool_*` 类事件

#### `commandExecutionEvents`

- 含义：Agent 能以结构化方式暴露 shell/command 执行事件
- 主要影响：
  - 前端是否展示命令执行卡片
  - 服务端是否提取命令级事件而不是只显示自然语言

#### `structuredPatch`

- 含义：Agent 可以输出机器可解析的 patch/edit 结果，而不是只给自然语言说明
- 主要影响：
  - 前端是否显示 diff / patch 预览
  - 服务端是否保存 patch artifact

### 6.3 交互流程类

#### `planMode`

- 含义：Agent 存在一类可识别的“规划模式”，其输出目标不同于直接执行
- 主要影响：
  - 前端是否显示“Plan”入口
  - 服务端是否需要把该模式编码为标准命令或启动参数

#### `approvalRequests`

- 含义：Agent 能明确提出审批、确认或继续执行请求
- 主要影响：
  - 前端是否显示批准/拒绝型交互控件
  - 服务端是否产出结构化 `input_requested` 子类型

### 6.4 输入与多模态类

#### `imageInput`

- 含义：Agent 支持图像作为会话输入的一部分
- 主要影响：
  - 前端是否显示图片上传入口
  - 服务端是否需要支持附件传输与引用

### 6.5 工作区类

#### `multiWorkspace`

- 含义：单个 Session 可显式关联或切换多个工作区
- 主要影响：
  - 前端是否显示工作区切换或范围选择器
  - 服务端是否维护多工作区上下文

---

## 7. Feature 对事件协议的影响

标准 feature 不只是静态标记，它还会影响 AgenLeash 的事件协议。

建议约定如下：

- `streamingText=true`
  - 应允许 `message_delta`
- `toolCallEvents=true`
  - 应允许 `tool_call_started`、`tool_call_updated`、`tool_call_completed`
- `commandExecutionEvents=true`
  - 应允许 `command_started`、`command_completed`
- `artifactOutput=true`
  - 应允许 `artifact_added`、`artifact_updated`
- `approvalRequests=true`
  - `input_requested` 应支持 `kind=approval`

如果某 feature 为 `false`，则对应事件不应被对外承诺为稳定契约。

---

## 8. 稳定性分级

建议把标准 feature 再分成三类稳定性：

- `stable`：可以被前后端稳定依赖
- `experimental`：允许使用，但前后端都应带 feature gate
- `deprecated`：仍可兼容读取，但不建议新 Adapter 继续声明

### 8.1 第一版建议分级

`stable`：

- `streamingText`
- `toolCallEvents`
- `structuredPatch`
- `planMode`
- `imageInput`
- `artifactOutput`
- `multiWorkspace`
- `approvalRequests`
- `fileReferences`
- `commandExecutionEvents`

当前无 `deprecated` 项。

---

## 9. 版本差异表达规则

同一个 feature 在不同版本上的差异，必须通过 `versionProfiles.overrides.features` 表达，而不是新起一套 key。

推荐：

```yaml
versionProfiles:
  - name: legacy
    match: "<1.10.0"
    overrides:
      features:
        structuredPatch: false
        toolCallEvents: false

  - name: stable
    match: ">=1.10.0 <2.0.0"
    overrides:
      features:
        structuredPatch: true
        toolCallEvents: true
```

不推荐：

```yaml
features:
  structuredPatchV2: true
  newToolCallEvents: true
```

如果 feature 语义真的发生根本变化，才应新增 key。

---

## 10. 前后端使用约定

### 10.1 AgenLeash 服务端

- 负责计算 `effective_features`
- 负责根据 feature 决定哪些事件可稳定对外暴露
- 负责在未知版本时回退到保守 feature 集合

### 10.2 AgenDash 客户端

- 只根据 `effective_features` 决定 UI 功能显隐
- 不应从 agent 名称推断 feature
- 不应假设同一家 agent 的所有版本功能一致

### 10.3 Adapter 作者

- 优先使用标准 key
- 自定义 key 必须 namespaced
- 新增通用 feature 时，应先补充 registry，再进入 adapter 配置

---

## 11. 推荐的首批实现范围

建议第一版优先支持以下 feature key：

- `streamingText`
- `toolCallEvents`
- `structuredPatch`
- `planMode`
- `approvalRequests`
- `imageInput`
- `artifactOutput`
- `multiWorkspace`

这几个 key 已经足以覆盖聊天 GUI、计划流、工具流、补丁流和多工作区这几条主路径。
