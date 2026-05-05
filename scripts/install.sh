#!/usr/bin/env sh
set -eu

REPO="${AGENLEASH_REPO:-agendash/AgenLeash}"
BASE_URL="${AGENLEASH_RELEASE_BASE_URL:-https://github.com/${REPO}/releases}"
VERSION="${AGENLEASH_VERSION:-latest}"
PREFIX="${AGENLEASH_PREFIX:-/usr/local}"
INSTALL_SERVICE="${AGENLEASH_INSTALL_SERVICE:-auto}"

log() {
  printf '%s\n' "$*"
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 1
  fi
}

detect_os() {
  case "$(uname -s)" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *)
      log "unsupported operating system: $(uname -s)"
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *)
      log "unsupported architecture: $(uname -m)"
      exit 1
      ;;
  esac
}

download() {
  src="$1"
  dst="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$src" -o "$dst"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$dst" "$src"
    return
  fi
  log "curl or wget is required"
  exit 1
}

generate_token() {
  if [ -n "${AGENLEASH_TOKEN:-}" ]; then
    printf '%s' "${AGENLEASH_TOKEN}"
    return
  fi
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]'
    return
  fi
  if [ -r /proc/sys/kernel/random/uuid ]; then
    awk 'NR == 1 { print tolower($0); exit }' /proc/sys/kernel/random/uuid
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    hex="$(openssl rand -hex 16 | tr '[:upper:]' '[:lower:]')"
    printf '%s-%s-%s-%s-%s\n' \
      "$(printf '%s' "$hex" | cut -c1-8)" \
      "$(printf '%s' "$hex" | cut -c9-12)" \
      "$(printf '%s' "$hex" | cut -c13-16)" \
      "$(printf '%s' "$hex" | cut -c17-20)" \
      "$(printf '%s' "$hex" | cut -c21-32)"
    return
  fi
  if [ -r /dev/urandom ] && command -v od >/dev/null 2>&1; then
    hex="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
    printf '%s-%s-%s-%s-%s\n' \
      "$(printf '%s' "$hex" | cut -c1-8)" \
      "$(printf '%s' "$hex" | cut -c9-12)" \
      "$(printf '%s' "$hex" | cut -c13-16)" \
      "$(printf '%s' "$hex" | cut -c17-20)" \
      "$(printf '%s' "$hex" | cut -c21-32)"
    return
  fi
  log "failed to generate a default UUID token; set AGENLEASH_TOKEN explicitly"
  exit 1
}

default_env_file() {
  if [ -n "${AGENLEASH_ENV_FILE_PATH:-}" ]; then
    printf '%s' "${AGENLEASH_ENV_FILE_PATH}"
    return
  fi
  if [ "$(id -u)" -eq 0 ]; then
    printf '/etc/agenleash/agenleash.env'
    return
  fi
  printf '%s/.config/agenleash/agenleash.env' "${HOME}"
}

render_template() {
  src="$1"
  dst="$2"
  binary_path="$3"
  env_file="$4"
  work_dir="$5"
  log_dir="$6"
  service_user="$7"
  service_group="$8"
  service_path="$9"

  sed \
    -e "s#__BINARY__#${binary_path}#g" \
    -e "s#__ENV_FILE__#${env_file}#g" \
    -e "s#__WORKING_DIRECTORY__#${work_dir}#g" \
    -e "s#__LOG_DIRECTORY__#${log_dir}#g" \
    -e "s#__USER__#${service_user}#g" \
    -e "s#__GROUP__#${service_group}#g" \
    -e "s#__PATH__#${service_path}#g" \
    "$src" > "$dst"
}

install_systemd_service() {
  examples_dir="$1"
  binary_path="$2"
  env_file="$3"
  data_dir="$4"
  service_user="${AGENLEASH_SERVICE_USER:-${SUDO_USER:-$(id -un)}}"
  service_group="${AGENLEASH_SERVICE_GROUP:-$(id -gn "${service_user}")}"
  unit_path="/etc/systemd/system/agenleash.service"

  if [ "$(id -u)" -ne 0 ]; then
    log "skip systemd: root is required"
    return
  fi

  install -d -m 0755 "$(dirname "$unit_path")"
  install -d -m 0755 "$data_dir"
  chown "${service_user}:${service_group}" "$data_dir"
  render_template \
    "${examples_dir}/agenleash.service" \
    "$unit_path" \
    "$binary_path" \
    "$env_file" \
    "$data_dir" \
    "/var/log/agenleash" \
    "$service_user" \
    "$service_group" \
    ""

  systemctl daemon-reload
  systemctl enable --now agenleash.service
  log "systemd service installed: ${unit_path}"
}

install_launchd_service() {
  examples_dir="$1"
  binary_path="$2"
  env_file="$3"
  data_dir="$4"
  plist_path="${HOME}/Library/LaunchAgents/io.agenleash.plist"
  service_path="${AGENLEASH_SERVICE_PATH:-${PATH}}"

  install -d -m 0755 "${HOME}/Library/LaunchAgents"
  install -d -m 0755 "${HOME}/Library/Logs/agenleash"
  install -d -m 0755 "$data_dir"
  render_template \
    "${examples_dir}/io.agenleash.plist" \
    "$plist_path" \
    "$binary_path" \
    "$env_file" \
    "$data_dir" \
    "${HOME}/Library/Logs/agenleash" \
    "$(id -un)" \
    "$(id -gn)" \
    "$service_path"

  launchctl bootout "gui/$(id -u)" "$plist_path" >/dev/null 2>&1 || true
  launchctl bootstrap "gui/$(id -u)" "$plist_path"
  launchctl enable "gui/$(id -u)/io.agenleash"
  log "launchd agent installed: ${plist_path}"
}

need_cmd tar
need_cmd install

OS="$(detect_os)"
ARCH="$(detect_arch)"
ASSET="agenleash_${OS}_${ARCH}.tar.gz"
if [ "$VERSION" != "latest" ]; then
  ASSET="agenleash_${VERSION}_${OS}_${ARCH}.tar.gz"
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

ARCHIVE_PATH="${TMPDIR}/${ASSET}"
if [ "$VERSION" = "latest" ]; then
  ARCHIVE_URL="${BASE_URL}/latest/download/${ASSET}"
else
  ARCHIVE_URL="${BASE_URL}/download/${VERSION}/${ASSET}"
fi

log "downloading ${ARCHIVE_URL}"
download "$ARCHIVE_URL" "$ARCHIVE_PATH"

EXTRACT_DIR="${TMPDIR}/extract"
install -d -m 0755 "$EXTRACT_DIR"
tar -xzf "$ARCHIVE_PATH" -C "$EXTRACT_DIR"

install -d -m 0755 \
  "${PREFIX}/bin" \
  "${PREFIX}/share/agenleash/adapters" \
  "${PREFIX}/share/agenleash/examples"

install -m 0755 "${EXTRACT_DIR}/bin/agenleash" "${PREFIX}/bin/agenleash"
for adapter in "${EXTRACT_DIR}"/share/agenleash/adapters/*.json; do
  install -m 0644 "$adapter" "${PREFIX}/share/agenleash/adapters/"
done
for example in "${EXTRACT_DIR}"/share/agenleash/examples/*; do
  install -m 0644 "$example" "${PREFIX}/share/agenleash/examples/"
done

ENV_FILE="$(default_env_file)"
DATA_DIR="${AGENLEASH_DATA_DIR:-${PREFIX}/var/lib/agenleash}"
install -d -m 0755 "$(dirname "$ENV_FILE")" "$DATA_DIR"

if [ ! -f "$ENV_FILE" ]; then
  TOKEN_VALUE="$(generate_token)"
  TOKEN_SOURCE="generated"
  if [ -n "${AGENLEASH_TOKEN:-}" ]; then
    TOKEN_SOURCE="custom"
  fi
  cat > "$ENV_FILE" <<EOF
AGENLEASH_TOKEN=${TOKEN_VALUE}
AGENLEASH_ADDR=${AGENLEASH_ADDR:-0.0.0.0:8081}
AGENLEASH_DATA_DIR=${DATA_DIR}

# Browser dashboard is disabled by default.
# AGENLEASH_ENABLE_WEB=true

# Point these to the host user's real agent data directories.
# AGENLEASH_CLAUDE_HOME=/home/you/.claude
# AGENLEASH_CODEX_HOME=/home/you/.codex

# Restrict managed sessions to trusted paths only.
# AGENLEASH_ALLOWED_WORKSPACE_ROOTS=/srv/workspaces,/srv/repos
EOF
  chmod 0600 "$ENV_FILE"
  log "created env file: ${ENV_FILE}"
else
  log "using existing env file: ${ENV_FILE}"
fi

case "$INSTALL_SERVICE" in
  auto)
    if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
      install_systemd_service "${PREFIX}/share/agenleash/examples" "${PREFIX}/bin/agenleash" "$ENV_FILE" "$DATA_DIR" || true
    elif [ "$OS" = "darwin" ] && command -v launchctl >/dev/null 2>&1; then
      install_launchd_service "${PREFIX}/share/agenleash/examples" "${PREFIX}/bin/agenleash" "$ENV_FILE" "$DATA_DIR" || true
    fi
    ;;
  systemd)
    install_systemd_service "${PREFIX}/share/agenleash/examples" "${PREFIX}/bin/agenleash" "$ENV_FILE" "$DATA_DIR"
    ;;
  launchd)
    install_launchd_service "${PREFIX}/share/agenleash/examples" "${PREFIX}/bin/agenleash" "$ENV_FILE" "$DATA_DIR"
    ;;
  none)
    ;;
  *)
    log "unsupported AGENLEASH_INSTALL_SERVICE=${INSTALL_SERVICE}"
    exit 1
    ;;
esac

log
log "agenleash installed to ${PREFIX}/bin/agenleash"
log "env file: ${ENV_FILE}"
if [ -n "${TOKEN_VALUE:-}" ]; then
  if [ "${TOKEN_SOURCE:-generated}" = "custom" ]; then
    log "token: using custom AGENLEASH_TOKEN from install environment"
  else
    log "generated token: ${TOKEN_VALUE}"
  fi
fi
log "next:"
log "  1. edit ${ENV_FILE}"
log "  2. start with: ${PREFIX}/bin/agenleash"
log "  3. or enable the installed service template"
log "custom token on first install:"
log "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | env AGENLEASH_TOKEN=your-own-token sh"
log "customize later:"
log "  edit ${ENV_FILE} and update AGENLEASH_TOKEN"
