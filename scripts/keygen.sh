#!/usr/bin/env bash
set -euo pipefail

# Determine script and root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
KEYS_DIR="${ROOT_DIR}/keys"

SERVICES=("web" "orchestrator" "payment-core" "mock-mfs" "mock-card" "mock-downstream")

echo "==> Generating JWS/JWE keys for Paylane services in ${KEYS_DIR}..."

mkdir -p "${KEYS_DIR}"

for SVC in "${SERVICES[@]}"; do
    SVC_DIR="${KEYS_DIR}/${SVC}"
    mkdir -p "${SVC_DIR}"

    # Signing keys (RSA 2048)
    if [[ ! -f "${SVC_DIR}/sign_private.pem" ]]; then
        echo "  [${SVC}] generating sign key pair..."
        openssl genpkey -algorithm RSA -out "${SVC_DIR}/sign_private.pem" -pkeyopt rsa_keygen_bits:2048 2>/dev/null
        openssl rsa -pubout -in "${SVC_DIR}/sign_private.pem" -out "${SVC_DIR}/sign_public.pem" 2>/dev/null
    fi

    # Encryption keys (RSA 2048)
    if [[ ! -f "${SVC_DIR}/enc_private.pem" ]]; then
        echo "  [${SVC}] generating encryption key pair..."
        openssl genpkey -algorithm RSA -out "${SVC_DIR}/enc_private.pem" -pkeyopt rsa_keygen_bits:2048 2>/dev/null
        openssl rsa -pubout -in "${SVC_DIR}/enc_private.pem" -out "${SVC_DIR}/enc_public.pem" 2>/dev/null
    fi
done

chmod -R 600 "${KEYS_DIR}"/*/*_private.pem 2>/dev/null || true
chmod -R 644 "${KEYS_DIR}"/*/*_public.pem 2>/dev/null || true

echo "==> All keys generated successfully!"
