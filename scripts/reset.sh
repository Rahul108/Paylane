#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"
echo "==> Resetting Paylane platform (wiping volumes & databases)..."
docker compose down -v
"${SCRIPT_DIR}/keygen.sh"
docker compose up -d --build
echo "==> Paylane reset complete!"
