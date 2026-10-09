#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=========================================================="
echo "           PAYLANE COMPREHENSIVE E2E VERIFICATION         "
echo "=========================================================="

echo "==> 1. Checking unauthenticated liveness endpoints..."

curl -sf http://localhost:3010/api/health >/dev/null && echo "  ✅ web (3010) live" || { echo "❌ web failed"; exit 1; }
curl -sf http://localhost:4010/livez >/dev/null && echo "  ✅ orchestrator (4010) live" || { echo "❌ orchestrator failed"; exit 1; }
curl -sf http://localhost:4011/livez >/dev/null && echo "  ✅ payment-core (4011) live" || { echo "❌ payment-core failed"; exit 1; }
curl -sf http://localhost:5011/livez >/dev/null && echo "  ✅ mock-mfs (5011) live" || { echo "❌ mock-mfs failed"; exit 1; }
curl -sf http://localhost:5012/livez >/dev/null && echo "  ✅ mock-card (5012) live" || { echo "❌ mock-card failed"; exit 1; }
curl -sf http://localhost:5013/livez >/dev/null && echo "  ✅ mock-downstream (5013) live" || { echo "❌ mock-downstream failed"; exit 1; }

echo "==> 2. Verifying 401 Unauthorized rejection without valid JWE..."
for PORT in 4010 4011 5011 5012 5013; do
    CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://localhost:${PORT}/health" || true)
    if [[ "$CODE" == "401" ]]; then
        echo "  ✅ Port ${PORT} correctly rejected unauthenticated request with 401"
    else
        echo "  ❌ Port ${PORT} returned unexpected status: ${CODE}"
        exit 1
    fi
done

echo "==> 3. Running JWE service mesh handshake test..."
(cd "${ROOT_DIR}/scripts/meshcheck" && KEYS_DIR="${ROOT_DIR}/keys" go run .)

echo "==> 4. Running Phase 3 End-to-End Payment Flow (MFS + Card)..."
(cd "${ROOT_DIR}/scripts/e2e_payment" && KEYS_DIR="${ROOT_DIR}/keys" go run .)

echo "==> 5. Running Phase 4 & Phase 5 End-to-End Flow (Bindings, UI-less, Saga Journeys & Needs Attention)..."
(cd "${ROOT_DIR}/scripts/e2e_saga" && KEYS_DIR="${ROOT_DIR}/keys" go run .)

echo "=========================================================="
echo "          🎉 ALL E2E VERIFICATION CHECKS PASSED!         "
echo "=========================================================="
