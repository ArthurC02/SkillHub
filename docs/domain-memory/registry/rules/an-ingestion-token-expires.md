---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:45:46.007817Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/token.go#L97-99"
    excerpt_sha256: "sha256:047daca93ecb9a1d6b3612d66614f88ea5a871dabaf45f707434750102959d16"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L75-94"
    excerpt_sha256: "sha256:f74e7042630fdfec76901313d2fd82fa2f76cbd9364dbb1e51a298e9c378cb81"
---
A trace ingestion token is a signed grant for one Run attempt; it expires after its time to live, and a tampered or expired token is refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- Verifying a token after it expired returns ErrTokenExpired.
