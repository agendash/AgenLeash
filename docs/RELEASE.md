# Release Process

## Excluded From Releases

Do not publish local runtime state:

- `.env`
- `tmp/`
- `dist/stage/`
- `docker-data/`
- SQLite files
- logs and pid files
- Playwright screenshots and traces

The repository uses [.gitignore](../.gitignore), [.dockerignore](../.dockerignore), and [scripts/build-dist.sh](../scripts/build-dist.sh) to keep release archives clean.

## Build Artifacts

```bash
VERSION=v0.1.0 ./scripts/build-dist.sh
```

Default targets:

- `darwin/arm64`
- `darwin/amd64`
- `linux/amd64`
- `linux/arm64`

Artifacts are written to `dist/`:

- `agenleash_<version>_<os>_<arch>.tar.gz`
- `agenleash_<os>_<arch>.tar.gz`
- `checksums.txt`

`dist/` is a generated directory and should not be committed.

## GitHub Release

Upload both archive forms for each target:

- versioned archives for fixed-version installs
- unversioned aliases for `latest` installer downloads

The installer downloads:

```text
latest/download/agenleash_<os>_<arch>.tar.gz
```

Fixed-version installs download:

```text
download/<tag>/agenleash_<tag>_<os>_<arch>.tar.gz
```

## Homebrew

Update [packaging/homebrew/agenleash.rb](../packaging/homebrew/agenleash.rb):

- `url`
- `sha256`

Then push the formula to the tap.

## Validation

Run before publishing:

```bash
go test ./...
GOOS=linux GOARCH=amd64 go test ./...
go build ./cmd/agenleash
docker build -t agenleash:release .
sh -n scripts/install.sh
bash -n scripts/build-dist.sh
```

Run `shellcheck scripts/*.sh` as well when `shellcheck` is available.
