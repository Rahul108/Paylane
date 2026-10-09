# Paylane Platform Overview

## Mission
Paylane is a local-only payment platform modeled as independent microservices in Go, mock payment gateways, a JWE-secured service mesh, and a test console.

## Architecture Principles
1. **Real Microservices, No Monorepo**: Each service is an independent repository/module with its own `go.mod`, Dockerfile, goose migrations, and tests.
2. **Database Per Service**: Strict schema isolation on MySQL 8. No cross-service SQL joins.
3. **Zero Shared Domain Code**: HTTP APIs (OpenAPI) are the contracts. Only `paylane-jwe` infrastructure library is shared.
4. **Money In Minor Units**: `int64` (poisha/cents) and 3-character ISO currency (`BDT`).
5. **JWE Service Mesh**: Sign-then-encrypt (JWS RS256 + JWE RSA-OAEP-256 / A256GCM).
6. **Application ULIDs**: 26-character monotonic ULIDs.
