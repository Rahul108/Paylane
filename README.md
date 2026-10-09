# Paylane

Local multi-service payment platform built with Go, Next.js, and MySQL. All inter-service communication is secured via JWE/JWS mutual encryption.

---

## 1. Installation & Running via Docker

### Prerequisites
- [Docker](https://docs.docker.com/get-docker/) & [Docker Compose v2+](https://docs.docker.com/compose/)

### Quick Start (One Command)
Start MySQL, Redis, and all 6 microservices:
```bash
docker compose up -d
```

### (Optional) Environment Customization
If you wish to customize port bindings or database passwords, copy the template:
```bash
cp deploy/.env.example deploy/.env
```
Then start the stack:
```bash
docker compose up -d
```

### Verify Running Stack
```bash
# Check container statuses and health checks
docker compose ps

# Run automated liveness, security (401), and JWE handshake verification
./scripts/e2e.sh
```

---

## 2. Checking Payment Journeys via UI

1. Open your browser and navigate to:
   **[http://localhost:3010](http://localhost:3010)**

2. In the **Interactive Test Console**:
   - **Choose Journey Type**: Select one of:
     - `payment_recharge`: Payment + Telecom airtime recharge
     - `ui_payment`: Hosted checkout redirect only
     - `uiless_payment`: 1-click tokenized payment without redirect
     - `payment_cashback`: Payment + promotional cashback grant
     - `payment_subscription`: Payment + offer/plan activation
   - **Payment Method**: Select `📱 MFS (bKash)` or `💳 Card (3DS)`
   - **Amount & Scenario**: Enter amount (BDT) and choose scenario (`Success Flow`, `Insufficient Balance`, etc.)
   - *(Optional)* Fill in journey-specific fields (e.g. mobile number for recharge, promo code for cashback, or token ID for UI-less).

3. Click **▶ Start Journey**:
   - **Visual Stepper**: Watch the stages progress dynamically:
     - *Recharge Flow*: `INITIATION` ➔ `PAYMENT` ➔ `RECHARGE` ➔ `COMPLETED`
     - *UI-less Flow*: `INITIATION` ➔ `BOUND CHARGE` ➔ `COMPLETED`
   - **Step Breakdown Table**: View real-time attempt counts, statuses (`SUCCEEDED`, `FAILED`), and downstream confirmation details.
   - **Payment State Machine**: Observe transitions from `CREATED` ➔ `PENDING` ➔ `CAPTURED`.
   - **Double-Entry Ledger Audit**: Inspect the generated immutable ledger records (Debit customer / Credit merchant in minor units).
   - **Live Activity Log**: Follow real-time JWE mesh network logs.

---

## 3. Checking Payment Journeys via APIs

### 1. Orchestrator Journey Status
Retrieve journey execution state and downstream step progress:
```bash
curl http://localhost:4010/journeys/{journey_id}
```
Example Response:
```json
{
  "journey_id": "JRN_01J9...",
  "journey_type": "payment_recharge",
  "status": "COMPLETED",
  "payment_id": "PAY_01J9...",
  "steps": [
    { "name": "PAYMENT", "status": "SUCCEEDED", "attempts": 1 },
    { "name": "RECHARGE", "status": "SUCCEEDED", "attempts": 1, "details": "01712345678" }
  ]
}
```

### 2. Payment-Core Canonical Payment State
Inspect the underlying payment state machine directly:
```bash
curl http://localhost:4011/payments/{payment_id}
```
Example Response:
```json
{
  "id": "PAY_01J9...",
  "amount": 50000,
  "currency": "BDT",
  "status": "CAPTURED",
  "method": "MFS"
}
```

### 3. Direct MySQL Ledger & Audit Inspection
Verify immutable double-entry ledger records directly in MySQL:
```bash
docker exec -it paylane-mysql mysql -u payment_core_user -ppayment_core_pass -D paylane_payment_core \
  -e "SELECT entry_type, account, amount, currency, reason, created_at FROM ledger_entries WHERE payment_id = '{payment_id}';"
```

Verify state transition event history:
```bash
docker exec -it paylane-mysql mysql -u payment_core_user -ppayment_core_pass -D paylane_payment_core \
  -e "SELECT from_status, to_status, reason, created_at FROM payment_events WHERE payment_id = '{payment_id}';"
```
