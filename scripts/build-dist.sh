#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
VERSION="${VERSION:-v0.1.0-dev}"
TARGETS="${TARGETS:-darwin/arm64 darwin/amd64 linux/amd64 linux/arm64}"

mkdir -p "${DIST_DIR}"
rm -rf "${DIST_DIR}/stage"
mkdir -p "${DIST_DIR}/stage"

copy_assets() {
  local stage_dir="$1"
  mkdir -p \
    "${stage_dir}/bin" \
    "${stage_dir}/share/agenleash/adapters" \
    "${stage_dir}/share/agenleash/examples"

  cp "${ROOT_DIR}/.env.example" "${stage_dir}/share/agenleash/examples/.env.example"
  cp "${ROOT_DIR}/packaging/systemd/agenleash.service" "${stage_dir}/share/agenleash/examples/agenleash.service"
  cp "${ROOT_DIR}/packaging/launchd/io.agenleash.plist" "${stage_dir}/share/agenleash/examples/io.agenleash.plist"
  cp "${ROOT_DIR}/README.md" "${stage_dir}/README.md"
  cp "${ROOT_DIR}/docs/INSTALL.md" "${stage_dir}/INSTALL.md"
  cp "${ROOT_DIR}"/adapters/*.json "${stage_dir}/share/agenleash/adapters/"
}

sha256_file() {
  local file="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file"
    return
  fi
  sha256sum "$file"
}

rm -f "${DIST_DIR}/checksums.txt"

for target in ${TARGETS}; do
  os="${target%/*}"
  arch="${target#*/}"
  stage_dir="${DIST_DIR}/stage/${VERSION}_${os}_${arch}"

  rm -rf "${stage_dir}"
  mkdir -p "${stage_dir}"
  copy_assets "${stage_dir}"

  (
    cd "${ROOT_DIR}"
    GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 \
      go build -trimpath -ldflags="-s -w" -o "${stage_dir}/bin/agenleash" ./cmd/agenleash
  )

  versioned_archive="${DIST_DIR}/agenleash_${VERSION}_${os}_${arch}.tar.gz"
  latest_alias="${DIST_DIR}/agenleash_${os}_${arch}.tar.gz"

  tar -C "${stage_dir}" -czf "${versioned_archive}" .
  cp "${versioned_archive}" "${latest_alias}"

  (
    cd "${DIST_DIR}"
    sha256_file "$(basename "${versioned_archive}")"
    sha256_file "$(basename "${latest_alias}")"
  ) >> "${DIST_DIR}/checksums.txt"
done

echo "artifacts written to ${DIST_DIR}"
