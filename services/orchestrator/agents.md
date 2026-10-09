# orchestrator Agent Guide

Workflow orchestrator executing payment journeys and downstream actions in Paylane.

## References
- Platform Guide: [agents.md](../../agents.md)
- Journey Architecture: [docs/services/orchestrator/](../../docs/services/orchestrator/)
- OpenAPI Spec: [api/openapi.yaml](api/openapi.yaml)

## Invariants
- The orchestrator has no payment rules or state machine logic; it delegates to `payment-core`.
- Steps follow configured definitions (`ui_payment`, `payment_recharge`, `payment_cashback`, etc.).
- Step execution records attempts, summaries, and idempotency keys (`journeyId + stepName`).
- Inter-service calls use JWE encryption/signing.
