# Runbook: Local Setup & Operation

## 1. Prerequisites
- Docker & Docker Compose
- OpenSSL (for key generation)
- Go 1.22+ and Node 20+ (for local development without Docker)

## 2. One-Command Startup
To start the entire platform with all services, MySQL, and Redis:
```bash
docker compose up -d
```

## 3. Verifying Health
```bash
./scripts/e2e.sh
```

## 4. Resetting State
To wipe all databases and recreate clean state:
```bash
./scripts/reset.sh
```

## 5. Teardown
```bash
docker compose down
```
