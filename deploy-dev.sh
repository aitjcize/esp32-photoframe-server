#!/bin/bash
# deploy-dev.sh - Deploy the dev add-on to Home Assistant for testing.
#
# Builds the add-on image LOCALLY (native arm64 on Apple Silicon = fast) and
# loads it straight into the HA host's Docker, instead of compiling on the
# (slow) Raspberry Pi. First-time installs still build once on-device.
#
# Requirements on the HA host:
#   - "Advanced SSH & Web Terminal" add-on with Protection mode OFF
#     (Settings -> Add-ons -> Advanced SSH & Web Terminal -> Protection mode),
#     otherwise the host `docker load` is blocked.
#
# Usage: ./deploy-dev.sh [homeassistant-host]   (default: root@homeassistant.local)
set -euo pipefail

HA_HOST="${1:-root@homeassistant.local}"
ADDON_NAME="esp32-photoframe-server-dev"
ADDON_SLUG="local_${ADDON_NAME}"
IMAGE="local/aarch64-addon-${ADDON_NAME}:dev"
ADDON_PORT="9608"
BUILD_FROM="ghcr.io/home-assistant/aarch64-base:3.19"
LOCAL_DIR="$(cd "$(dirname "$0")" && pwd)"
REMOTE_DIR="/addons/${ADDON_NAME}"

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
  amd64: ghcr.io/home-assistant/amd64-base:3.19
args:
  ADDON_PORT: "${ADDON_PORT}"
EOF
ssh "${HA_HOST}" "ha apps reload"

# First-time install: the supervisor must build once on-device (no image yet).
if ! ssh "${HA_HOST}" "ha apps info ${ADDON_SLUG}" >/dev/null 2>&1; then
  echo "=== First-time install (one slow on-device build) ==="
  ssh "${HA_HOST}" "ha apps install ${ADDON_SLUG}"
  ssh "${HA_HOST}" "ha apps start ${ADDON_SLUG}" || true
  echo "=== Installed. Subsequent runs use the fast local-build path. ==="
  exit 0
fi

# Preflight: host Docker must be reachable (SSH add-on Protection mode OFF).
if ! ssh "${HA_HOST}" "docker version" >/dev/null 2>&1; then
  echo "ERROR: cannot access Docker on ${HA_HOST}." >&2
  echo "Turn OFF Protection mode on the 'Advanced SSH & Web Terminal' add-on," >&2
  echo "then restart that add-on and re-run this script." >&2
  exit 1
fi

echo "=== Building ${IMAGE} (linux/arm64) on $(uname -m) ==="
docker buildx build "${LOCAL_DIR}" \
  --tag "${IMAGE}" \
  --file "${LOCAL_DIR}/Dockerfile" \
  --platform linux/arm64 \
  --build-arg BUILD_VERSION=dev \
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

echo "=== Transferring image to ${HA_HOST} (docker save | gzip | ssh docker load) ==="
docker save "${IMAGE}" | gzip | ssh "${HA_HOST}" "docker load"

echo "=== Restarting add-on ${ADDON_SLUG} (no rebuild) ==="
ssh "${HA_HOST}" "ha apps restart ${ADDON_SLUG}" \
  || ssh "${HA_HOST}" "ha apps start ${ADDON_SLUG}"

echo "=== Done. Add-on status: ==="
ssh "${HA_HOST}" "ha apps info ${ADDON_SLUG} | grep -E '^(state|version):'" || true
echo "Logs: ssh ${HA_HOST} 'ha apps logs ${ADDON_SLUG}'"
