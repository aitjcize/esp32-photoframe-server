#!/bin/bash
# deploy-dev.sh - Deploy the dev add-on to Home Assistant for testing.
#
# Builds the add-on image LOCALLY (native arm64 on Apple Silicon = fast) and
# loads it straight into the HA host's Docker, instead of compiling on the
# (slow) Raspberry Pi. First-time installs still build once on-device.
#
# Requirements on the HA host:
#   - HAOS host debug SSH enabled on port 22222 (your key in the boot CONFIG
#     partition's authorized_keys). This reaches the real Docker daemon
#     directly, bypassing the SSH add-on's Protection-mode wrapper.
#   - The SSH add-on (port 22) for rsync to /addons and `ha` CLI commands.
#
# Usage: ./deploy-dev.sh [homeassistant-host]   (default: root@homeassistant.local)
set -euo pipefail

HA_HOST="${1:-root@homeassistant.local}"
ADDON_NAME="esp32-photoframe-server-dev"
ADDON_SLUG="local_${ADDON_NAME}"
IMAGE="local/aarch64-addon-${ADDON_NAME}:dev"
ADDON_PORT="9608"
BUILD_FROM="ghcr.io/home-assistant/aarch64-base:3.21"
LOCAL_DIR="$(cd "$(dirname "$0")" && pwd)"
REMOTE_DIR="/addons/${ADDON_NAME}"

# HAOS host debug SSH (port 22222): direct access to the real Docker daemon.
HOST_SSH_PORT="${HOST_SSH_PORT:-22222}"
HOST_DOCKER_SSH=(ssh -p "${HOST_SSH_PORT}" -o StrictHostKeyChecking=accept-new "${HA_HOST}")

echo "=== Syncing add-on config to ${HA_HOST}:${REMOTE_DIR} ==="
ssh "${HA_HOST}" "mkdir -p ${REMOTE_DIR}"
rsync -az --delete \
  --exclude '.git' \
  --exclude 'node_modules' \
  --exclude 'webapp/node_modules' \
  --exclude 'webapp/dist' \
  --exclude '*.db' \
  --exclude 'data/' \
  --exclude 'bin/' \
  --exclude '.DS_Store' \
  "${LOCAL_DIR}/" "${HA_HOST}:${REMOTE_DIR}/"

# Dev-ify config.yaml (slug/name/version/port) and write a dev build.yaml.
ssh "${HA_HOST}" "cd ${REMOTE_DIR} && \
  sed -i 's/^image:.*$/# image removed for local build/' config.yaml && \
  sed -i 's/^slug:.*/slug: \"${ADDON_NAME}\"/' config.yaml && \
  sed -i 's/^name:.*/name: \"ESP32 PhotoFrame Server (Dev)\"/' config.yaml && \
  sed -i 's/^version:.*/version: \"dev\"/' config.yaml && \
  sed -i 's/panel_title:.*/panel_title: PhotoFrame Dev/' config.yaml && \
  sed -i 's#9607/tcp: 9607#9607/tcp: ${ADDON_PORT}#' config.yaml && \
  sed -i 's/^  ADDON_PORT: .*/  ADDON_PORT: \"${ADDON_PORT}\"/' config.yaml"
ssh "${HA_HOST}" "cat > ${REMOTE_DIR}/build.yaml" <<EOF
build_from:
  aarch64: ${BUILD_FROM}
  amd64: ghcr.io/home-assistant/amd64-base:3.21
args:
  ADDON_PORT: "${ADDON_PORT}"
EOF
# Get the supervisor to pick up the synced add-on. `ha apps reload` refreshes the
# git-based store repos but does NOT rescan the LOCAL add-ons folder, so a
# brand-new local add-on stays invisible ("... does not exist in the store")
# until the supervisor restarts. Reload first (cheap, enough for updates); if the
# add-on still isn't in the store, restart the supervisor to force a local
# rescan.
in_store() { ssh "${HA_HOST}" "ha apps info ${ADDON_SLUG}" >/dev/null 2>&1; }

echo "=== Reloading add-on store ==="
ssh "${HA_HOST}" "ha apps reload" >/dev/null 2>&1 || true
sleep 2
if ! in_store; then
  echo "=== New local add-on: restarting supervisor so it detects ${ADDON_SLUG} ==="
  ssh "${HA_HOST}" "ha supervisor restart" >/dev/null 2>&1 || true
  for attempt in $(seq 1 30); do
    in_store && break
    sleep 4
  done
fi
if ! in_store; then
  echo "ERROR: ${ADDON_SLUG} never appeared in the add-on store." >&2
  echo "Check that files landed under the supervisor's local add-ons dir and that" >&2
  echo "${REMOTE_DIR}/config.yaml is valid after dev-ification:" >&2
  echo "  ssh ${HA_HOST} 'ls -la ${REMOTE_DIR} && ha supervisor restart'" >&2
  exit 1
fi

# First-time install: the supervisor must build once on-device (no image yet).
# The add-on is now in the store (poll above), so distinguish "installed" from
# "store-only" by state: a not-yet-installed add-on reports `state: unknown`.
if ssh "${HA_HOST}" "ha apps info ${ADDON_SLUG}" 2>/dev/null | grep -qE '^state: *unknown'; then
  echo "=== First-time install (one slow on-device build) ==="
  ssh "${HA_HOST}" "ha apps install ${ADDON_SLUG}"
  ssh "${HA_HOST}" "ha apps start ${ADDON_SLUG}" || true
  echo "=== Installed. Subsequent runs use the fast local-build path. ==="
  exit 0
fi

# Preflight: host Docker reachable via the HAOS debug SSH (port 22222).
if ! "${HOST_DOCKER_SSH[@]}" "docker version" >/dev/null 2>&1; then
  echo "ERROR: cannot reach Docker on ${HA_HOST}:${HOST_SSH_PORT}." >&2
  echo "Enable HAOS debug SSH on port ${HOST_SSH_PORT} and authorize your key" >&2
  echo "(authorized_keys on the boot CONFIG partition)." >&2
  exit 1
fi

echo "=== Building ${IMAGE} (linux/arm64) on $(uname -m) ==="
# Resolve the converter's current npm release so the install layer's cache
# busts exactly when a new version ships (see Dockerfile). A failed lookup
# aborts the deploy: falling back to a stable string like "latest" would let
# Docker reuse the cached layer and silently ship an old converter.
if ! command -v npm >/dev/null 2>&1; then
  echo "ERROR: npm is required on this machine to resolve the current" >&2
  echo "@aitjcize/epaper-image-convert release (install Node.js/npm)" >&2
  exit 1
fi
CONVERTER_VERSION="$(npm view @aitjcize/epaper-image-convert version)"
if [ -z "${CONVERTER_VERSION}" ]; then
  echo "ERROR: could not resolve @aitjcize/epaper-image-convert version from npm" >&2
  exit 1
fi
echo "=== Using epaper-image-convert ${CONVERTER_VERSION} ==="

docker buildx build "${LOCAL_DIR}" \
  --tag "${IMAGE}" \
  --file "${LOCAL_DIR}/Dockerfile" \
  --platform linux/arm64 \
  --build-arg BUILD_VERSION=dev \
  --build-arg CONVERTER_VERSION="${CONVERTER_VERSION}" \
  --build-arg BUILD_ARCH=aarch64 \
  --build-arg ADDON_PORT="${ADDON_PORT}" \
  --build-arg BUILD_FROM="${BUILD_FROM}" \
  --label io.hass.version=dev \
  --label io.hass.arch=aarch64 \
  --label io.hass.type=app \
  --label io.hass.name="ESP32 PhotoFrame Server (Dev)" \
  --label io.hass.description="Image server for ESP32 PhotoFrame" \
  --label io.hass.url="https://github.com/aitjcize/esp32-photoframe-server" \
  --load

echo "=== Transferring image to ${HA_HOST}:${HOST_SSH_PORT} (docker save | gzip | docker load) ==="
docker save "${IMAGE}" | gzip | "${HOST_DOCKER_SSH[@]}" "docker load"

echo "=== Restarting add-on ${ADDON_SLUG} (no rebuild) ==="
ssh "${HA_HOST}" "ha apps restart ${ADDON_SLUG}" \
  || ssh "${HA_HOST}" "ha apps start ${ADDON_SLUG}"

echo "=== Done. Add-on status: ==="
ssh "${HA_HOST}" "ha apps info ${ADDON_SLUG} | grep -E '^(state|version):'" || true
echo "Logs: ssh ${HA_HOST} 'ha apps logs ${ADDON_SLUG}'"
