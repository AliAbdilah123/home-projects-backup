#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROJECT_NAME="omnichannel-chat-hub"
TARGET_DIR="/usr/share/nginx/html/projects/${PROJECT_NAME}"

cd "$ROOT_DIR/frontend"
npm run build

sudo mkdir -p "$TARGET_DIR"
sudo cp -r dist/* "$TARGET_DIR/"
sudo chown -R nginx:nginx "$TARGET_DIR"
sudo chmod -R o+rX "$TARGET_DIR"

echo "Frontend deployed to $TARGET_DIR"
