# AgenLeash

AgenLeash 是给 `AgenDash` 或其他前端消费的轻量级 Code Agent Runtime Gateway。

它负责：

- 启动并托管 `codex`、`claude`、`opencode`、`cursor`、`gemini`、`grok`、`pi`、`acpx` 这类本地 code agent 进程
- 统一输出会话、消息、工作区与事件流
- 发现本机已有的 `.codex` / `.claude` 历史会话
- 通过 HTTP + WebSocket 暴露远程可控的会话接口

## 快速开始

### 1. 独立安装

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

安装脚本首次运行会自动生成一个 UUID 格式的 `AGENLEASH_TOKEN`，写入 `agenleash.env`，并在终端打印出来。

如果你想在安装时自定义 token，可以这样执行：

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | env AGENLEASH_TOKEN=my-custom-token sh
```

安装后，编辑 `agenleash.env`：

```dotenv
AGENLEASH_TOKEN=<安装脚本生成的 UUID，或你自定义的值>
AGENLEASH_ADDR=0.0.0.0:8081
AGENLEASH_PORT=28081
# AGENLEASH_ENABLE_WEB=true
AGENLEASH_CLAUDE_HOME=/home/you/.claude
AGENLEASH_CODEX_HOME=/home/you/.codex
AGENLEASH_OPENCODE_HOME=/home/you/.local/share/opencode
AGENLEASH_CURSOR_HOME=/home/you/.cursor
AGENLEASH_GEMINI_HOME=/home/you/.gemini
AGENLEASH_GROK_HOME=/home/you/.grok
AGENLEASH_PI_HOME=/home/you/.pi
AGENLEASH_ACPX_HOME=/home/you/.acpx
AGENLEASH_ALLOWED_WORKSPACE_ROOTS=/srv/workspaces,/srv/repos
```

然后启动：

```bash
agenleash
```

### 2. Docker 安装

```bash
cp .env.example .env
docker compose up -d --build
```

请把 `.env` 里的这些目录改成宿主机真实路径：

- `AGENLEASH_CLAUDE_HOST_DIR`
- `AGENLEASH_CODEX_HOST_DIR`
- `AGENLEASH_OPENCODE_HOST_DIR`
- `AGENLEASH_CURSOR_HOST_DIR`
- `AGENLEASH_GEMINI_HOST_DIR`
- `AGENLEASH_GROK_HOST_DIR`
- `AGENLEASH_PI_HOST_DIR`
- `AGENLEASH_ACPX_HOST_DIR`
- `AGENLEASH_XDG_CONFIG_HOST_DIR`
- `AGENLEASH_XDG_CACHE_HOST_DIR`
- `AGENLEASH_XDG_STATE_HOST_DIR`
- `AGENLEASH_LOCAL_BIN_HOST_DIR`
- `AGENLEASH_WORKSPACE_HOST_DIR`
- `AGENLEASH_AGENT_BIN_HOST_DIR`

如果宿主机的 `8081` 已经被 AgenDash 或其他服务占用，请把 `AGENLEASH_PORT` 改成别的值，例如 `28081`。

如果你希望容器内也能启动托管的 code agent，需要把 `claude` / `codex` / `opencode` / `agent` / `gemini` / `grok` / `pi` / `acpx` 这类可执行文件放进 `AGENLEASH_AGENT_BIN_HOST_DIR` 或 `AGENLEASH_LOCAL_BIN_HOST_DIR`，也可以基于本仓库 Dockerfile 再做一层派生镜像。

如果你是手动编辑 `.env`，最简单的做法是先执行一次 `uuidgen`，再把结果填到 `AGENLEASH_TOKEN=`。

### 3. Homebrew 安装

仓库已经提供 formula 模板 [packaging/homebrew/agenleash.rb](packaging/homebrew/agenleash.rb)。

发布时：

1. 打 tag 并上传 release 产物。
2. 更新 formula 中的 `url` / `sha256`。
3. 推到你的 tap 仓库后执行 `brew install <tap>/agenleash`。

Homebrew 首次安装也会自动生成 UUID token，写到 `$(brew --prefix)/etc/agenleash/agenleash.env`。如果你想自定义，可以在安装前传入：

```bash
AGENLEASH_TOKEN=my-custom-token brew install <tap>/agenleash
```

如果你用的是 macOS `launchd`，又希望 AgenLeash 在后台也能找到 `claude` / `codex` 这类用户级 CLI，安装时可以顺手把当前 shell 的 PATH 一起传进去：

```bash
AGENLEASH_SERVICE_PATH="$PATH" \
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

## 统计页

浏览器统计页默认关闭。

如果你需要打开 `/stats`，显式加上：

```dotenv
AGENLEASH_ENABLE_WEB=true
```

启用后可以打开：

```text
http://<host>:<port>/stats
```

页面会让你输入 `AGENLEASH_TOKEN`，然后展示：

- 总会话数、活跃数、待 review 数
- Adapter 分布
- `claudecode` 主会话 / `subagents` 拆分
- Top workspaces

如果你更想直接取 JSON，也可以调用：

```text
GET /api/v1/stats?top=20
```

## 目录映射建议

无论是裸机安装还是 Docker，建议统一成下面这套约定：

- Agent 历史目录
  - Claude: `AGENLEASH_CLAUDE_HOME`
  - Codex: `AGENLEASH_CODEX_HOME`
  - OpenCode: `AGENLEASH_OPENCODE_HOME`
  - Cursor: `AGENLEASH_CURSOR_HOME`
  - Gemini: `AGENLEASH_GEMINI_HOME`
  - Grok: `AGENLEASH_GROK_HOME`
  - Pi: `AGENLEASH_PI_HOME`
  - ACPX: `AGENLEASH_ACPX_HOME`
- 允许运行的工作区根目录
  - `AGENLEASH_ALLOWED_WORKSPACE_ROOTS`
- AgenLeash 自身数据目录
  - `AGENLEASH_DATA_DIR`

推荐映射：

| 用途 | 宿主机路径示例 | 容器内路径示例 |
| --- | --- | --- |
| Claude 历史与登录态 | `/home/you/.claude` | `/home/agenleash/.claude` |
| Codex 历史与登录态 | `/home/you/.codex` | `/home/agenleash/.codex` |
| OpenCode 数据 | `/home/you/.local/share/opencode` | `/home/agenleash/.local/share/opencode` |
| Cursor 数据 | `/home/you/.cursor` | `/home/agenleash/.cursor` |
| Gemini 数据 | `/home/you/.gemini` | `/home/agenleash/.gemini` |
| Grok 数据 | `/home/you/.grok` | `/home/agenleash/.grok` |
| Pi 数据 | `/home/you/.pi` | `/home/agenleash/.pi` |
| ACPX 数据 | `/home/you/.acpx` | `/home/agenleash/.acpx` |
| XDG 配置 | `/home/you/.config` | `/home/agenleash/.config` |
| XDG 缓存 | `/home/you/.cache` | `/home/agenleash/.cache` |
| XDG 状态 | `/home/you/.local/state` | `/home/agenleash/.local/state` |
| 用户级 CLI | `/home/you/.local/bin` | `/home/agenleash/.local/bin` |
| 代码工作区 | `/home/you/Workspace` | `/workspaces` |
| Agent 可执行文件 | `/opt/agent-bin` | `/opt/agent-bin` |
| AgenLeash 数据 | `/var/lib/agenleash` | `/var/lib/agenleash` |

## 安全建议

- 默认必须配置 `AGENLEASH_TOKEN`
- 只有显式设置 `AGENLEASH_ALLOW_NO_TOKEN=true` 才会关闭鉴权
- 浏览器统计页默认关闭，只有显式设置 `AGENLEASH_ENABLE_WEB=true` 才会暴露 `/stats`
- 对外部署时务必设置 `AGENLEASH_ALLOWED_WORKSPACE_ROOTS`
- 发布包、Docker 构建上下文和 Homebrew 安装都应排除 `tmp/`

## 文档

- 安装与部署：[docs/INSTALL.md](docs/INSTALL.md)
- 发布流程：[docs/RELEASE.md](docs/RELEASE.md)
- Adapter Schema：[docs/ADAPTER_SCHEMA.md](docs/ADAPTER_SCHEMA.md)
- Feature Registry：[docs/FEATURE_REGISTRY.md](docs/FEATURE_REGISTRY.md)
