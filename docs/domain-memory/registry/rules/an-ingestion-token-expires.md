---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/token.go#L97-99"
    excerpt_sha256: "sha256:047daca93ecb9a1d6b3612d66614f88ea5a871dabaf45f707434750102959d16"
---
A trace ingestion token is a signed grant for one Run attempt; it expires after its time to live, and a tampered or expired token is refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- Verifying a token after it expired returns ErrTokenExpired.
