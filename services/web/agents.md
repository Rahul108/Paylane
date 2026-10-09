# web Agent Guide

Frontend customer application, server-side BFF, and test console.

## References
- Platform Guide: [agents.md](../../agents.md)
- Web & BFF Architecture: [docs/services/web/](../../docs/services/web/)

## Invariants
- Browser sessions talk only to Next.js BFF using standard cookies.
- Server-side BFF signs and encrypts calls to `orchestrator` via JWE.
- Never expose private RSA keys to the browser.
- Browser redirect parameters are never trusted for payment completion.
