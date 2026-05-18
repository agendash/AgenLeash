# 安装与部署

## 1. 环境变量

最少需要配置：

| 变量 | 说明 |
| --- | --- |
| `AGENLEASH_TOKEN` | API 鉴权 token，生产环境必填 |
| `AGENLEASH_PORT` | Docker Compose 对外发布的宿主机端口，默认 `8081` |
| `AGENLEASH_ADDR` | 服务监听地址，默认 `0.0.0.0:8081` |
| `AGENLEASH_ENABLE_WEB` | 是否启用浏览器统计页 `/stats`，默认关闭 |
| `AGENLEASH_DATA_DIR` | AgenLeash 自身 SQLite 与状态目录 |
| `AGENLEASH_CLAUDE_HOME` | Claude 历史目录 |
| `AGENLEASH_CODEX_HOME` | Codex 历史目录 |
| `AGENLEASH_OPENCODE_HOME` | OpenCode 历史目录 |
| `AGENLEASH_CURSOR_HOME` | Docker / 本地 adapter 运行 Cursor CLI 时使用的状态目录 |
| `AGENLEASH_GEMINI_HOME` | Docker / 本地 adapter 运行 Gemini CLI 时使用的状态目录 |
| `AGENLEASH_GROK_HOME` | Docker / 本地 adapter 运行 Grok CLI 时使用的状态目录 |
| `AGENLEASH_PI_HOME` | Docker / 本地 adapter 运行 Pi CLI 时使用的状态目录 |
| `AGENLEASH_ACPX_HOME` | Docker / 本地 adapter 运行 ACPX 时使用的状态目录 |
| `AGENLEASH_ALLOWED_WORKSPACE_ROOTS` | 允许托管 agent 启动的工作区根目录列表 |

补充说明：

- 没有 `AGENLEASH_TOKEN` 时，服务会拒绝启动
- 只有显式设置 `AGENLEASH_ALLOW_NO_TOKEN=true` 才会关闭鉴权
- 浏览器统计页默认关闭，只有显式设置 `AGENLEASH_ENABLE_WEB=true` 才会暴露 `/stats`
- `AGENLEASH_ALLOWED_WORKSPACE_ROOTS` 支持逗号、换行或路径分隔符分隔

## 2. 独立安装

### 2.1 远程安装

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

首次安装时，如果你没有显式传入 `AGENLEASH_TOKEN`，脚本会自动生成一个 UUID token，写入默认 `agenleash.env`，并在安装输出里打印给你。

常用可选参数：

```bash
AGENLEASH_VERSION=v0.1.0 \
AGENLEASH_PREFIX=/usr/local \
AGENLEASH_INSTALL_SERVICE=systemd \
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

脚本会做这些事：

1. 下载对应平台的 release 压缩包。
2. 安装 `agenleash` 二进制与内置 adapters。
3. 创建默认 `agenleash.env`，并自动写入一个 UUID token。
4. 按平台尝试安装 `systemd` 或 `launchd` 服务。

如果你希望在安装时自定义 token：

```bash
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | env AGENLEASH_TOKEN=my-custom-token sh
```

如果安装后再修改，直接编辑生成的 `agenleash.env`，更新 `AGENLEASH_TOKEN=...` 即可。

### 2.2 手动安装

```bash
go build -o ./tmp/bin/agenleash ./cmd/agenleash
cp .env.example .env
AGENLEASH_ENV_FILE=.env ./tmp/bin/agenleash
```

## 3. Docker 安装

### 3.1 准备目录映射

建议统一成下面的容器内路径：

| 宿主机 | 容器内 |
| --- | --- |
| `~/.claude` | `/home/agenleash/.claude` |
| `~/.codex` | `/home/agenleash/.codex` |
| `~/.local/share/opencode` | `/home/agenleash/.local/share/opencode` |
| `~/.cursor` | `/home/agenleash/.cursor` |
| `~/.gemini` | `/home/agenleash/.gemini` |
| `~/.grok` | `/home/agenleash/.grok` |
| `~/.pi` | `/home/agenleash/.pi` |
| `~/.acpx` | `/home/agenleash/.acpx` |
| `~/.config` | `/home/agenleash/.config` |
| `~/.cache` | `/home/agenleash/.cache` |
| `~/.local/state` | `/home/agenleash/.local/state` |
| `~/.local/bin` | `/home/agenleash/.local/bin` |
| `~/Workspace` | `/workspaces` |
| `./agent-bin` | `/opt/agent-bin` |
| `./docker-data` | `/var/lib/agenleash` |

### 3.2 启动

```bash
cp .env.example .env
docker compose up -d --build
```

至少要改这些变量：

```dotenv
AGENLEASH_TOKEN=<安装脚本生成的 UUID，或你自定义的值>
AGENLEASH_PORT=28081
# AGENLEASH_ENABLE_WEB=true
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
AGENLEASH_WORKSPACE_HOST_DIR=/absolute/path/to/Workspace
AGENLEASH_AGENT_BIN_HOST_DIR=/absolute/path/to/agent-bin
```

如果宿主机本身已经有 AgenDash 在占用 `8081`，把 `AGENLEASH_PORT` 改成 `28081`、`38081` 等空闲端口即可；容器内的 `AGENLEASH_ADDR=0.0.0.0:8081` 不需要改。

如果你是手动维护 `.env`，可以先执行一次 `uuidgen`，把结果填到 `AGENLEASH_TOKEN=`。

### 3.3 托管 agent 的额外要求

只挂载 `.claude` / `.codex` / OpenCode 数据目录时，AgenLeash 可以发现已有历史会话。

如果还要在容器里直接启动托管的 `claude` / `codex` / `opencode` / `agent` / `gemini` / `grok` / `pi` / `acpx`，需要额外满足之一：

1. 把 agent 二进制挂载到 `/opt/agent-bin` 或 `/home/agenleash/.local/bin`
2. 基于 `Dockerfile` 构建派生镜像，在镜像里安装对应 CLI

## 4. systemd 开机自启

模板文件见 [packaging/systemd/agenleash.service](../packaging/systemd/agenleash.service)。

示例：

```bash
sudo install -d /etc/agenleash
sudo cp .env.example /etc/agenleash/agenleash.env
sudo install -m 0644 packaging/systemd/agenleash.service /etc/systemd/system/agenleash.service
sudo systemctl daemon-reload
sudo systemctl enable --now agenleash
```

上线前请改：

- `User=` / `Group=`
- `Environment=AGENLEASH_ENV_FILE=...`
- `WorkingDirectory=...`

## 4.1 统计页

浏览器统计页默认关闭。

如果你确实需要 `/stats`，先在 env 里加上：

```dotenv
AGENLEASH_ENABLE_WEB=true
```

然后可以打开：

```text
http://<host>:<port>/stats
```

页面本身不直接暴露数据，打开后需要输入 `AGENLEASH_TOKEN` 才会拉取统计。

统计接口也可以单独调用：

```text
GET /api/v1/stats?top=20
```

默认会展示：

- 总会话数与 review 状态
- Adapter 分布
- `claudecode` 主会话 / `subagents` 拆分
- Top workspaces

## 5. macOS launchd 开机自启

模板文件见 [packaging/launchd/io.agenleash.plist](../packaging/launchd/io.agenleash.plist)。

示例：

```bash
mkdir -p ~/Library/LaunchAgents
cp packaging/launchd/io.agenleash.plist ~/Library/LaunchAgents/io.agenleash.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/io.agenleash.plist
launchctl enable gui/$(id -u)/io.agenleash
```

建议搭配用户级 `~/.config/agenleash/agenleash.env` 使用。

如果要让 `launchd` 托管后的 AgenLeash 也能找到 `claude` / `codex` 这类用户级 CLI，可以在安装时显式传入：

```bash
AGENLEASH_SERVICE_PATH="$PATH" \
curl -fsSL https://raw.githubusercontent.com/agendash/AgenLeash/main/scripts/install.sh | sh
```

## 6. Homebrew 发布

Formula 模板见 [packaging/homebrew/agenleash.rb](../packaging/homebrew/agenleash.rb)。

推荐流程：

1. 执行 `VERSION=v0.1.0 ./scripts/build-dist.sh`
2. 发布 GitHub Release
3. 更新 formula 中的 `url` 和 `sha256`
4. 推到独立 tap 仓库
5. 用户执行 `brew install <tap>/agenleash`

Homebrew 首次安装时也会自动生成 UUID token 并写入 `$(brew --prefix)/etc/agenleash/agenleash.env`。如果要自定义，可以在首次安装前执行：

```bash
AGENLEASH_TOKEN=my-custom-token brew install <tap>/agenleash
```
