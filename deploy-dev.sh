#!/bin/bash
# deploy-dev.sh - Deploy local add-on to Home Assistant for development
# Usage: ./deploy-dev.sh [homeassistant-host]

set -e

HA_HOST="${1:-root@homeassistant.local}"
ADDON_NAME="esp32-photoframe-server-dev"
LOCAL_DIR="$(cd "$(dirname "$0")" && pwd)"
REMOTE_DIR="/addons/${ADDON_NAME}"

echo "=== Deploying dev add-on to ${HA_HOST}:${REMOTE_DIR} ==="

# Create remote directory
ssh "${HA_HOST}" "mkdir -p ${REMOTE_DIR}"

# Sync files (excluding unnecessary files)
rsync -avz --delete \
  --exclude '.git' \
  --exclude 'node_modules' \
  --exclude 'frontend/node_modules' \
  --exclude 'frontend/dist' \
  --exclude '*.db' \
  --exclude 'data/' \
  --exclude 'bin/' \
  --exclude '.DS_Store' \
  "${LOCAL_DIR}/" "${HA_HOST}:${REMOTE_DIR}/"

# Modify config.yaml for dev: remove image line, change slug/name/port
ssh "${HA_HOST}" "cd ${REMOTE_DIR} && \
  sed -i 's/^image:.*$/# image removed for local build/' config.yaml && \
  sed -i 's/^slug:.*/slug: \"esp32-photoframe-server-dev\"/' config.yaml && \
  sed -i 's/^name:.*/name: \"ESP32 PhotoFrame Server (Dev)\"/' config.yaml && \
  sed -i 's/^version:.*/version: \"dev\"/' config.yaml && \
  sed -i 's/panel_title:.*/panel_title: PhotoFrame Dev/' config.yaml && \
  sed -i 's/9607\\/tcp: 9607/9607\\/tcp: 9608/' config.yaml"

# Create dev build.yaml with port 9608
ssh "${HA_HOST}" "cat > ${REMOTE_DIR}/build.yaml" << 'EOF'
build_from:
  aarch64: ghcr.io/home-assistant/aarch64-base:3.19
  amd64: ghcr.io/home-assistant/amd64-base:3.19
  armv7: ghcr.io/home-assistant/armv7-base:3.19
args:
  ADDON_PORT: "9608"
EOF

echo ""
echo "=== Deployment complete! ==="
echo ""
echo "Triggering Supervisor to reload and rebuild..."

# Reload add-ons to detect the new/updated local add-on
ssh "${HA_HOST}" "ha addons reload"

# Check if add-on is already installed
ADDON_SLUG="local_${ADDON_NAME}"
if ssh "${HA_HOST}" "ha addons info ${ADDON_SLUG}" &>/dev/null; then
    echo "Add-on already installed, rebuilding..."
    ssh "${HA_HOST}" "ha addons rebuild ${ADDON_SLUG}"
else
    echo "Installing add-on for the first time..."
    ssh "${HA_HOST}" "ha addons install ${ADDON_SLUG}"
fi

echo ""
echo "Waiting for build to complete..."
sleep 5

# Start the add-on
ssh "${HA_HOST}" "ha addons start ${ADDON_SLUG}" || true

echo ""
echo "=== Done! ==="
echo "Check logs with: ssh ${HA_HOST} 'ha addons logs ${ADDON_SLUG}'"
