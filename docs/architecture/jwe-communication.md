# JWE Service-to-Service Security

## Overview
Every inter-service request and response payload is secured using **Sign-then-Encrypt**:

```
Sender:
  Payload -> Sign with Sender Private Key (JWS RS256)
          -> Encrypt with Receiver Public Key (JWE RSA-OAEP-256 + A256GCM)

Receiver:
  Decrypt with Receiver Private Key
  -> Verify Signature with Sender Public Key
  -> Validate Claims (aud, exp, iat, jti replay check)
  -> Process Payload
```

## Claims Schema
- `iss`: Issuer service name (`web`, `orchestrator`, `payment-core`, etc.)
- `aud`: Destination service name
- `iat`: Timestamp (UTC seconds)
- `exp`: Expiration timestamp (60 seconds standard TTL)
- `jti`: Monotonic ULID token identifier (checked against replay cache)
