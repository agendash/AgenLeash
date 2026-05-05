# 产品需求文档（PRD）：分布式智能体控制基座 AgenLeash

## 1. 项目定位

**AgenLeash** 是一个入口型、轻量级的分布式代理服务，用于在宿主机上安全托管各类 Coding Agent，并向 **AgenDash** 暴露统一的远程会话能力。

它本身不实现 AI 推理，也不承担任务编排职责；其核心价值是把不同 Code Agent 的运行时差异、会话标识差异和工作区差异收敛成一套稳定的服务端抽象，供聊天窗口式 GUI 消费。

共享语音后端由 `AgenSense` 提供，AgenLeash 不自己承担语音识别或语音合成的主链路；如果某条业务需要语音输入/输出，也应先经过 `AgenSense` 再进入 AgenDash 或 AgenLeash 的会话流。

当前第一版本地联调建议默认监听在 `127.0.0.1:8081`，避免与 `AgenSense` 的 `127.0.0.1:8080` 默认端口冲突。

其核心职责包括：

- 以受控方式启动、托管和回收 Agent 进程
- 为依赖交互式终端的 Agent 提供兼容运行时
- 将 Agent 原始输出归一化为结构化会话事件
- 维护 `Session`、`Conversation`、`Workspace` 三者之间的映射关系
- 提供断线重连、状态观测、会话恢复和多节点接入能力

一句话定义：**AgenLeash 是 AgenDash 面向远端宿主机的 Code Agent Runtime 与 Session Event Gateway。**

---

## 2. 设计目标与非目标

### 2.1 设计目标

- 支持 `claudecode` 等依赖交互式运行环境的 Agent 在远端宿主机稳定运行
- 面向聊天窗口式 GUI 输出结构化事件，而不是把“终端渲染”作为客户端主协议
- 为不同 Code Agent 统一抽象会话、版本特性、原生会话 ID、工作区和恢复语义
- 支持客户端断线重连后无缝恢复会话上下文
- 为分布式节点部署提供统一认证、会话管理和服务发现能力
- 以 JSON 结构化事件作为主协议，终端 ANSI 解析只作为旧版兼容和调试辅助

### 2.2 非目标

- 不在 AgenLeash 内部实现具体 AI 能力或对话逻辑
- 不把终端模拟器作为 AgenDash 的默认前端形态
- 不在服务端拼装 GUI，不把服务端做成“界面渲染器”
- 不在 MVP 阶段实现复杂调度系统、任务编排器或多租户资源平台

---

## 3. 核心技术架构

### 3.1 运行时兼容层（Runtime Compatibility Layer）

#### 核心原则

不同 Code Agent 对运行环境的要求不同。AgenLeash 对外提供统一会话模型，但内部必须允许多种运行模式并存。

建议的运行模式包括：

- **STDIO 模式**：用于只要求标准输入输出的命令行 Agent
- **PTY 模式**：用于依赖真实 TTY 的交互式 CLI Agent
- **原生接入模式**：用于未来支持具备稳定 API 或事件流协议的 Agent

#### 当前要求

第一版应优先打通 **STDIO / non-interactive** 路径，用最小成本验证 Adapter、Workspace、Conversation 和结构化事件主链路。

PTY 不再作为第一阶段阻塞项，而是放到后续兼容阶段，用于承接那些明确依赖真实 TTY 的 agent/version。

推荐实现：

- Go：`github.com/creack/pty`
- Python：标准库 `pty` 或等价方案

#### 设计约束

- 若某个 agent/version 同时支持 `stdio` 与 `pty`，应默认优先选择 `stdio/non-interactive`
- PTY 是**服务端内部兼容层**，不是对外主协议
- 对依赖 TTY 的 Agent，不能以普通管道替代 PTY，否则会导致交互模式退化
- 若某 Adapter 声明运行时依赖终端尺寸，服务端需支持可选的运行时 `resize`
- 单个 Session 独占一个运行时实例和一个 Agent 子进程
- 运行时读写必须采用异步模型，避免慢连接阻塞 Session 主循环

### 3.2 会话事件协议（Session Event Protocol）

#### 控制通道（HTTPS + JSON）

用于低频控制与查询：

- 节点认证
- 获取 Session 列表
- 查询节点状态
- 启动新会话
- 查询会话详情
- 修改运行配置

#### 数据通道（WebSocket + JSON Event）

用于双向会话事件流传输，面向聊天式 GUI，而不是面向终端模拟器。

结构化 JSON 事件是主路径；ANSI / 光标 / 终端控制序列只用于兼容尚未提供结构化事件的旧版 Adapter。

**服务端下行事件**建议包括：

- `session_snapshot`：当前 Session 快照
- `message_started`：Agent 开始输出一条新消息
- `message_delta`：消息增量
- `message_completed`：消息结束
- `input_requested`：服务端判断当前需要用户继续输入
- `state_changed`：Session 状态变化
- `conversation_bound`：绑定到原生会话 ID
- `workspace_updated`：工作区信息更新
- `sync_end`：断线缓存补发完成
- `raw_chunk`：可选，仅调试模式下暴露原始输出片段

**客户端上行命令**建议包括：

- `user_message`：用户发送自然语言或指令输入
- `interrupt`：请求中断当前 Agent 执行
- `heartbeat`：连接保活
- `request_raw_stream`：请求开启调试级原始流
- `runtime_resize`：可选，仅在 Adapter 明确声明需要时启用

#### 设计原则

- 对外主协议一律以 **JSON 结构化事件** 为中心
- 原始字节流只作为底层事实来源或调试能力，不作为 GUI 的主要消费接口
- 客户端不应依赖 ANSI、光标位置或终端控制序列来推断会话含义

### 3.3 适配器层（Adapter Layer）

#### 目标

不同 Code Agent 在以下方面存在显著差异：

- 启动命令不同
- 不同版本之间能力边界不同
- 环境变量不同
- 功能特性集合不同
- 原生会话 ID 生成方式不同
- 工作区绑定方式不同
- 恢复语义不同
- 输出格式和可解析程度不同

因此，AgenLeash 必须通过 Adapter 层做入口级解耦，而不是将这些差异写死在核心服务里。

#### 每个 Adapter 至少定义

- `name`：适配器标识
- `runtime_mode`：`pty` / `stdio` / `native`
- `agent_family`：Agent 家族标识
- `schema_version`：Adapter 配置格式版本
- `entrypoint`：启动命令
- `args`：默认参数
- `env`：环境变量注入
- `pre_check`：启动前检查
- `cwd_policy`：工作目录约束策略
- `version_strategy`：版本识别策略
- `version_profiles`：按版本范围声明的能力差异
- `conversation_mode`：会话绑定方式
- `workspace_mode`：工作区识别方式
- `capabilities`：能力声明
- `features`：功能特性声明
- `event_parser`：原始输出到结构化事件的转换策略

#### `capabilities` 建议字段

- `supports_resume`
- `supports_interrupt`
- `requires_tty`
- `requires_runtime_resize`
- `supports_raw_debug`
- `supports_workspace_switch`
- `supports_native_conversation_id`

#### `features` 建议字段

`features` 用于描述某个 agent/version 对外暴露的功能特性，而不是底层运行时能力。例如：

- `streaming_text`
- `tool_call_events`
- `structured_patch`
- `plan_mode`
- `image_input`
- `artifact_output`
- `multi_workspace`

#### Adapter 职责

- 启动并托管对应 Agent
- 识别当前 Agent 版本，并匹配正确的版本能力档位
- 识别当前版本可用的 feature 集合，并向 Session 元数据暴露
- 从原始输出、日志文件、环境回显或外部命令中提取原生 `conversation_id`
- 将 Agent 输出归一化为 AgenLeash 会话事件
- 将 `user_message` 等服务端标准命令转换为 Agent 可接受的输入形式
- 探测和上报工作区信息
- 声明该 Agent 的恢复策略和工作区约束

### 3.4 本机历史会话发现与 Session Meta 索引缓存

#### 目标

当 AgenDash 请求某个 leash 的“全部 agents / conversations”时，返回结果不能只包含 **当前由 AgenLeash 托管的活跃 Session**，还必须包含 **当前宿主机上本地 Code Agent 自己保存的历史会话索引**。

这里的缓存目标必须保持克制：AgenLeash 只缓存“能定位原始 conversation/session 的最小 meta 索引”，不复制 transcript 正文，不生成摘要，也不把消息条数之类的派生信息写入本地 SQLite。

这类数据来源类似 CodeIsland 与本地 agent 的互动方式：

- 读取本地 agent 自己维护的 session index / thread index / transcript / sqlite
- 解析出该 agent 的历史 `session / conversation / workspace`
- 只提取 `session id / conversation id / workspace / source path`
- 归一化成 AgenLeash 的统一 meta 索引目录

#### 设计要求

- AgenLeash 必须把返回结果区分为：
  - `managed`：当前由 AgenLeash 启动或托管的 Session
  - `discovered`：从本机 agent 历史目录或本地数据库中发现的 Session
- `discovered` Session 只需要补齐最小索引信息：
  - `adapter`
  - `native_conversation_id`
  - `workspace_path`
  - `workspace_root`
  - `git_branch`
  - `detail_path`（统一表示原始 session 文件路径或数据库 locator）
- 历史索引的组织方式应显式体现：
  - 一个 `workspace` 下可以关联多个 `conversation/session` meta 记录
  - AgenLeash 缓存的是“索引关系”，不是 conversation 正文副本
- SQLite 中的 discovered 缓存只保留可定位字段，不保留：
  - `summary`
  - `message_count`
  - 任意逐条消息内容
- 发现逻辑必须按 agent family 区分，不允许假设所有 agent 共享同一套本地文件布局

#### 刷新策略

- AgenLeash 启动时应主动刷新一次本机历史 meta index
- 每次 `GET /api/v1/sessions` 或 `GET /api/v1/sessions/{id}` 前，应触发一次轻量刷新
- 轻量刷新必须带节流，不能把每次读请求都退化成一次全量本地历史扫描；服务端应至少支持可配置的最小刷新间隔
- 第一版实现建议暴露：
  - `AGENLEASH_HISTORY_REFRESH_INTERVAL`，例如 `30s`
  - `AGENLEASH_SESSION_PERSIST_INTERVAL`，例如 `2s`
- 刷新结果必须以 session meta index 的形式写入本地 SQLite，作为 AgenDash 的稳定读取缓存
- 即使某个 agent 当前没有运行进程，只要本地历史仍存在，也应出现在 discovered index 中
- 若需要查看 conversation 详情，应回到 agent 原始 session 文件或原生存储；AgenLeash 的 SQLite 不承担正文归档职责
### 3.5 Conversation 与 Workspace 适配模型

#### 背景

不同 Code Agent 并不共享同一套会话模型。有些 Agent 一个进程对应一个会话，有些 Agent 会生成可恢复的原生对话 ID，有些则只能依赖“进程仍然存活”来实现恢复。

同样，不同 Agent 对 Workspace 的理解也不同：

- 有的只依赖 `cwd`
- 有的实际绑定的是 Git 仓库根目录
- 有的会在内部维护独立的 project/session 目录

因此，AgenLeash 必须把以下三个概念显式区分：

- **Session**：一个实际运行中的 Agent 实例
- **Conversation**：用户与 Agent 的逻辑会话线程
- **Workspace**：Agent 当前执行任务的工作上下文

#### 服务端统一字段

每个 Session 至少应维护以下标准字段：

- `session_id`
- `adapter`
- `native_conversation_id`
- `start_mode`：`new` / `resume`
- `resume_strategy`
- `workspace_path`
- `workspace_root`
- `workspace_fingerprint`
- `git_root`
- `git_branch`
- `state`
- `created_at`
- `last_seen`

#### 适配规则

- MVP 阶段默认假设“一个 Session 只承载一个活跃 Conversation”
- 若某 Agent 暴露稳定的原生 `conversation_id`，Adapter 必须将其绑定到 `session_id`
- 若某 Agent 不提供稳定会话标识，则 `resume_strategy` 需标记为 `process_only`
- 会话恢复时，服务端应同时校验 `adapter + native_conversation_id + workspace_fingerprint`
- 若原生会话标识可复用但工作区不一致，服务端应拒绝自动恢复或要求显式确认

#### Workspace Fingerprint 建议

`workspace_fingerprint` 建议由以下信息组合生成：

- 规范化后的 `cwd`
- Git 仓库根目录
- Git 远端仓库标识（若可获取）
- 适配器自定义的项目标识

这样可以避免“同一 conversation_id 被错误恢复到另一个项目目录”。

### 3.6 版本识别、Feature 与能力矩阵

#### 背景

同一个 Code Agent 的不同版本，往往并不共享完全一致的行为模型。例如：

- 某个旧版本只支持 PTY 模式，不支持稳定恢复
- 某个新版本新增原生 `conversation_id`
- 某个版本开始支持中断，但输出格式发生变化
- 某个版本新增 `plan_mode` 或 `structured_patch`
- 某个版本更改了工作区切换方式或日志路径

因此，AgenLeash 不能只按“agent 名称”做适配，必须同时考虑 **agent family + agent version**。

#### 设计要求

- Adapter 定义应分为“家族级基础配置”和“版本档位覆盖配置”
- `capabilities` 不应被视为一个静态常量，而应允许按版本范围覆盖
- `features` 也不应被视为一个静态常量，而应允许按版本范围覆盖
- `conversation_mode`、`workspace_mode`、`event_parser` 也必须支持按版本切换
- 当版本无法识别时，服务端必须回退到 `unknown` 或 `safe` 档位，而不是乐观启用高级能力

#### 版本来源建议

服务端可按优先级尝试识别版本：

1. 启动请求显式提供的 `agent_version_hint`
2. 执行二进制的 `--version` 或等价命令
3. Adapter 自定义的 `detect_version` 钩子
4. 从启动输出或元信息文件中提取版本

#### 匹配结果

版本识别完成后，服务端应产出统一结果：

- `agent_family`
- `resolved_version`
- `matched_profile`
- `effective_capabilities`
- `effective_features`

该结果应记录在 Session 元数据中，并用于后续事件解析、恢复策略选择和故障排查。

---

## 4. 关键功能需求细节

### 4.1 零断点会话接管（Session Persistence）

#### 要求

- 当客户端断开连接时，Agent 进程必须继续在后台运行
- Session 生命周期不能绑定单个前端连接
- 同一个 Session 后续可以重新接入

#### 缓存模型

服务端为每个 Session 至少维护两类缓存：

- **事件缓存（主缓存）**：缓存最近的结构化会话事件，供聊天式 GUI 重连同步
- **原始缓存（可选）**：缓存最近的原始输出片段，供调试或 Adapter 诊断使用

事件缓存建议至少支持以下任一策略：

- 最近 `N` 条事件
- 最近 `1 MB` 左右的事件序列化结果

#### 重连同步

客户端重新接入时，服务端应按以下顺序处理：

1. 校验会话可用性和权限
2. 发送 `session_snapshot`
3. 补发最近事件缓存
4. 发送 `sync_end`
5. 切换到实时事件流

### 4.2 侧向状态监测（Side-car Monitoring）

#### 原则

状态监测必须是**非侵入式**的：

- 不修改 Agent 输出
- 不要求 Agent 强制实现自定义协议
- 不让客户端依赖终端语义做业务判断

#### 监测来源

Session 状态可综合以下信息判断：

- 运行时进程状态
- Adapter 的原始输出解析结果
- 工作区与 Git 探测结果
- 最近事件活动时间

#### 状态模型建议

建议 Session 至少包含以下状态：

- `starting`
- `awaiting_user`
- `responding`
- `running_task`
- `idle`
- `exited`
- `error`
- `zombie`

#### 判定示例

- Agent 正在持续输出正文内容，可标记为 `responding`
- Agent 输出暂停且等待用户继续输入，可标记为 `awaiting_user`
- Agent 正在执行工具或长任务，但暂未形成最终回复，可标记为 `running_task`
- 进程退出时标记为 `exited`
- 进程状态与缓存状态不一致时标记为 `zombie`

### 4.3 Conversation 绑定与恢复

#### 绑定要求

- Agent 启动后，Adapter 应在可接受时间窗口内尝试绑定原生 `conversation_id`
- 一旦绑定成功，服务端应生成 `conversation_bound` 事件
- 若超时仍未提取到原生会话标识，需明确记录原因并落入 `process_only` 恢复策略

#### 恢复模式

服务端应支持以下 `start_mode`：

- `new`：启动一个全新会话
- `resume`：按现有原生会话标识尝试恢复

#### 恢复校验

当请求 `resume` 时，至少校验：

- Adapter 类型是否一致
- `native_conversation_id` 是否存在
- `workspace_fingerprint` 是否匹配
- 当前 Agent 是否声明 `supports_resume=true`

### 4.4 Workspace 探测与约束

服务端应尽可能实时维护以下工作区信息：

- 当前工作目录绝对路径
- 逻辑工作区根目录
- Git 仓库根目录
- 当前分支名
- 最近活跃时间

#### 获取方式

- 启动参数中的 `cwd`
- Adapter 自定义探测逻辑
- Git 命令探测，例如 `git rev-parse --show-toplevel`
- 必要时读取 Agent 产生的元信息文件

#### 约束要求

- 应支持配置允许访问的工作区根目录白名单
- 应阻止 Session 在未授权路径下启动
- 若 Agent 在运行时切换到新工作区，服务端应产出 `workspace_updated` 事件

### 4.5 局域网自发现（Auto Discovery）

#### 协议

基于 **mDNS / DNS-SD** 广播服务。

#### 服务定义

- Service Name：`_agenleash._tcp`
- TXT 元数据建议包含：
  - `node_name`
  - `version`
  - `auth_required=true`
  - `port`

#### 目标

AgenDash 在局域网内应能够自动发现可接入节点，并展示节点名称、版本和认证要求，降低手工配置成本。

---

## 5. 接口契约（API Contracts）

### 5.1 获取 Session 列表

- **Method**：`GET /api/v1/sessions`
- **Auth**：`X-AgenLeash-Token`
- **可选查询参数**：`limit=<n>`，用于嵌入端或低内存客户端只拉取按优先级排序后的前 N 条 session，避免一次性返回过大的历史列表

返回字段建议包括：

- `id`
- `adapter`
- `origin`
- `state`
- `pid`
- `created_at`
- `last_seen`
- `connected_clients`
- `capabilities`
- `features`
- `conversation`
- `workspace`

其中：

- `conversation` 建议包含：
  - `native_id`
  - `start_mode`
  - `resume_strategy`
- session 顶层建议额外包含：
  - `detail_path`
- `workspace` 建议包含：
  - `cwd`
  - `root`
  - `fingerprint`
  - `git_root`
  - `git_branch`

该接口的语义应为：

- 返回 `managed + discovered` 的合并视图，而不是只返回当前 runtime manager 内存中的活跃 session
- 若本次请求触发了本机 agent meta index 刷新，则响应应反映刷新后的最新结果
- 当本机历史 meta index 规模较大时，应补充 `origin` / `adapter` / `workspace` / `limit` / `cursor` 等筛选与分页能力，避免一次性返回超大 JSON

### 5.2 获取单个 Session 详情

- **Method**：`GET /api/v1/sessions/{session_id}`
- **Auth**：`X-AgenLeash-Token`

用途：

- 读取单条会话的稳定详情
- 为 AgenDash 的会话详情页、恢复入口或 HMI 卡片展开态提供数据
- 该接口也应在返回前触发一次本机历史刷新
- 对于 `discovered` 会话，该接口返回的仍应是 meta 详情，而不是 transcript、摘要或消息统计
### 5.3 启动新任务

- **Method**：`POST /api/v1/agent/start`
- **Auth**：`X-AgenLeash-Token`

请求体示例：

```json
{
  "adapter": "claudecode",
  "cwd": "/path/to/project",
  "agent_version_hint": "1.12.0",
  "start_mode": "new",
  "conversation_id": null,
  "args": ["--agent-mode"]
}
```

恢复会话请求示例：

```json
{
  "adapter": "claudecode",
  "cwd": "/path/to/project",
  "agent_version_hint": "1.12.0",
  "start_mode": "resume",
  "conversation_id": "conv_xxx",
  "args": []
}
```

处理流程：

1. 校验 Token 和 Adapter 配置
2. 识别 Agent 版本并匹配版本档位
3. 校验 `cwd` 与工作区约束
4. 执行 `pre_check`
5. 创建运行时实例（如 PTY）
6. 启动 Agent 进程
7. 绑定 Conversation 与 Workspace 元数据
8. 返回 `session_id`

### 5.4 实时会话事件通道

- **Endpoint**：`WS /ws/v1/sessions/{session_id}/events?token=xxx`

#### 服务端下行事件

- `session_snapshot`
- `message_started`
- `message_delta`
- `message_completed`
- `input_requested`
- `state_changed`
- `conversation_bound`
- `workspace_updated`
- `sync_end`
- `raw_chunk`（可选）

#### 客户端上行命令

- `user_message`
- `interrupt`
- `heartbeat`
- `request_raw_stream`
- `runtime_resize`（仅可选）

#### `user_message` 示例

```json
{
  "msg_type": "user_message",
  "message_id": "msg_user_001",
  "content": "帮我检查一下这个仓库里的测试失败原因"
}
```

#### `message_delta` 示例

```json
{
  "msg_type": "message_delta",
  "message_id": "msg_agent_001",
  "role": "assistant",
  "delta": "我先检查测试输出和最近变更。"
}
```

#### `conversation_bound` 示例

```json
{
  "msg_type": "conversation_bound",
  "session_id": "sess_123",
  "adapter": "claudecode",
  "native_conversation_id": "conv_xxx",
  "resume_strategy": "native_id+workspace"
}
```

#### `workspace_updated` 示例

```json
{
  "msg_type": "workspace_updated",
  "session_id": "sess_123",
  "cwd": "/path/to/project",
  "workspace_root": "/path/to/project",
  "git_root": "/path/to/project",
  "git_branch": "main"
}
```

#### 原始流说明

- `raw_chunk` 仅用于调试或兼容性回退
- 若暴露 `raw_chunk`，内容应使用 Base64 编码并显式标注来源 Adapter
- GUI 主流程不得依赖 `raw_chunk` 渲染

---

## 6. 安全与资源约束

### 6.1 鉴权

- 每个 HTTP 请求和 WebSocket 请求都必须校验 `X-AgenLeash-Token` 或等价 Token
- MVP 阶段可采用单固定 Token
- 后续可扩展为节点级 Token、用户级 Token 或短时签名令牌

### 6.2 运行隔离

- 支持以配置方式指定运行 Agent 的 `UID/GID`
- 默认不应以 `root` 身份直接运行 Agent
- 应限制可访问工作目录范围，避免任意路径执行
- 应限制可使用的 Adapter 白名单，避免任意命令注入

### 6.3 资源限制

单 Session 至少支持以下限制：

- 最大内存占用
- 最大运行时长
- 最大空闲时长
- 最大并发 Session 数

限制触发后，服务端应记录原因，并将 Session 标记为 `error` 或 `exited`。

---

## 7. 存储与可观测性建议

### 7.1 持久化

第一版就应使用 SQLite 或等价轻量数据库，持久化以下信息：

- Session 元数据
- discovered session meta index
- Adapter 配置与映射关系
- 原生 Conversation 绑定关系
- Workspace 指纹与 Git 信息
- 最近状态
- 关键生命周期事件

明确不建议持久化：

- conversation 正文副本
- 自动生成的摘要
- 消息条数统计

### 7.2 日志与诊断

建议至少区分三类日志：

- 节点运行日志
- Session 生命周期日志
- Adapter 启动、解析与异常日志

### 7.3 指标建议

建议暴露以下基础指标：

- 当前 Session 数
- 活跃连接数
- 平均事件同步时延
- Conversation 恢复成功率
- Workspace 不匹配拒绝次数
- Adapter 解析失败次数
- 运行时读写错误次数
- 进程异常退出次数

---

## 8. 开发阶段划分

### 第一阶段（MVP）

目标：打通单节点、单 Token、聊天式 GUI 接入链路。

范围：

- STDIO / non-interactive 运行时
- WebSocket JSON 事件流
- 基础 Session 生命周期管理
- 本机历史 session discovery 与 SQLite meta index 缓存
- 单固定 Token 鉴权
- 至少一个 non-interactive Adapter 打通主链路
- AgenDash 可通过聊天式 GUI 流畅接入并控制远端 Agent

### 第二阶段（增强）

目标：补齐兼容层、恢复能力、适配能力和工作区安全性。

范围：

- PTY 运行时兼容层
- Adapter 配置文件支持
- SQLite 会话持久化
- 结构化事件缓存与断线重连同步
- Conversation 绑定与恢复
- Workspace 指纹与路径约束
- 基础状态识别与工作区探测

### 第三阶段（生态）

目标：支持多节点和多 Agent 扩展。

范围：

- mDNS 自动发现
- 系统负载与节点信息上报
- 多客户端同步观察
- 更细粒度的权限控制
- 多 Adapter 能力矩阵
- 可选原始调试流和兼容性回退机制
