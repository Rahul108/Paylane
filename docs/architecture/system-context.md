# System Context & Boundaries

```mermaid
graph TD
    User([End User / Browser]) -->|Cookie / HTTP| Web["web (:3010)"]
    Web -->|JWE| Orch["orchestrator (:4010)"]
    Orch -->|JWE| Core["payment-core (:4011)"]
    Orch -->|JWE| Downstream["mock-downstream (:5013)"]
    Core -->|JWE| MFS["mock-mfs (:5011)"]
    Core -->|JWE| Card["mock-card (:5012)"]
    MFS -.->|JWE Webhooks| Core
    Card -.->|JWE Webhooks| Core
```

## Service Boundaries
- **web (Port 3010)**: Next.js BFF + UI + Test Console. Server-side JWE signing/encryption.
- **orchestrator (Port 4010)**: Saga orchestrator. Defines journey steps, records attempts, handles retries.
- **payment-core (Port 4011)**: Payment domain authority. State transitions, double-entry ledger, outbox events, provider adapters.
- **mock-mfs (Port 5011)**: Simulates MFS PGW (bKash) with hosted PIN/OTP pages and webhooks.
- **mock-card (Port 5012)**: Simulates Card PGW with hosted forms, 3DS OTP, and tokenization.
- **mock-downstream (Port 5013)**: Simulates downstream recharge, cashback, and offer subscriptions.
