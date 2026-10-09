# orchestrator

Journey orchestrator microservice for the Paylane platform.

## Features
- Dynamic journey execution (payment + recharge / cashback / subscription)
- Step tracking with retry logic and `NEEDS_ATTENTION` recovery state
- Asynchronous outbox event subscriber
- JWE/JWS mutual authentication

## Running locally
```bash
PORT=4010 KEYS_DIR=../../keys go run ./cmd/orchestrator
```
