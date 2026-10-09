# payment-core Agent Guide

This service is the single source of truth for payment status, transactions, and state transitions in Paylane.

## References
- Platform Guide: [agents.md](../../agents.md)
- Architecture & State Machine: [docs/services/payment-core/state-machine.md](../../docs/services/payment-core/state-machine.md)
- Ledger & Refunds: [docs/services/payment-core/ledger-and-refunds.md](../../docs/services/payment-core/ledger-and-refunds.md)
- OpenAPI Spec: [api/openapi.yaml](api/openapi.yaml)

## Invariants
- Only `payment-core` mutates payment state.
- All monetary amounts are `int64` minor units (poisha).
- All IDs are 26-character ULIDs generated in Go.
- Every state transition records a double-entry ledger item and an outbox event in the same DB transaction.
- Inter-service calls require signed-then-encrypted JWE/JWS.
