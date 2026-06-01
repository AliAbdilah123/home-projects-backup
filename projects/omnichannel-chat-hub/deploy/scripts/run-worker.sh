#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/worker"

if [[ -f "$ROOT_DIR/deploy/env/worker.env" ]]; then
  # shellcheck disable=SC2046
  export $(grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "$ROOT_DIR/deploy/env/worker.env" | xargs)
fi

exec node dist/index.js
