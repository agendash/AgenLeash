# 发布流程

## 1. 先清理可发布内容

仓库里不应该把下面这些内容打进发布包：

- `tmp/`
- `.env`
- SQLite 文件
- 本机日志、pid、调试产物

已经通过以下文件做了隔离：

- [.gitignore](../.gitignore)
- [.dockerignore](../.dockerignore)
- [scripts/build-dist.sh](../scripts/build-dist.sh)

## 2. 生成 release 产物

```bash
VERSION=v0.1.0 ./scripts/build-dist.sh
```

默认会生成：

- `darwin/arm64`
- `darwin/amd64`
- `linux/amd64`
- `linux/arm64`

产物位于 `dist/`：

- `agenleash_<version>_<os>_<arch>.tar.gz`
- `agenleash_<os>_<arch>.tar.gz`
- `checksums.txt`

## 3. 上传 GitHub Release

建议把每个平台的两个文件都上传：

- 带版本号的包，给固定版本安装使用
- 不带版本号的别名包，给 `latest` 安装脚本使用

`curl | sh` 默认下载：

- `latest/download/agenleash_<os>_<arch>.tar.gz`

固定版本安装下载：

- `download/<tag>/agenleash_<tag>_<os>_<arch>.tar.gz`

## 4. 更新 Homebrew Formula

更新 [packaging/homebrew/agenleash.rb](../packaging/homebrew/agenleash.rb) 中的：

- `url`
- `sha256`

然后推到你的 tap。

## 5. 验证项

发版前至少验证：

1. `go test ./...`
2. `GOOS=linux GOARCH=amd64 go test ./...`
3. `docker build -t agenleash:release .`
4. `shellcheck scripts/install.sh`，如果本机安装了 `shellcheck`
