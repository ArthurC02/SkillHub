---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L305-311"
    excerpt_sha256: "sha256:0fcc239e2b9a7844b2e22c59ed8eeed66bab19404458f5da21329eb72f50d81d"
  - resource: "apps/platform/internal/trial/improvement/suggest_store_test.go#L40-66"
    excerpt_sha256: "sha256:cc73bb66b0ceba6f8a592c4371428aaa668b34c26d71026d79d8b3fc1f4cde87"
---
Informational findings are left out of the suggestion digest, so a disclosure cannot be quoted as proof of a problem.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- An info finding's excerpt is not in the digest.
