# payment-core

Payment engine microservice for the Paylane platform.

## Features
- State machine: `CREATED -> PENDING -> AUTHORIZED -> CAPTURED -> REFUNDED`
- Idempotency with application-generated ULIDs
- Immutable double-entry ledger
- Provider adapters (`mfs`, `card`)
- Outbox event streaming
- JWE/JWS mutual authentication

## Running locally
```bash
PORT=4011 KEYS_DIR=../../keys go run ./cmd/payment-core
```
