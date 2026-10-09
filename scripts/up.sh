#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Ensure keys exist
if [[ ! -d "${ROOT_DIR}/keys/payment-core" ]]; then
    "${SCRIPT_DIR}/keygen.sh"
fi

cd "${ROOT_DIR}"
echo "==> Starting Paylane platform with docker compose..."
docker compose up -d --build
echo "==> Stack started successfully!"
