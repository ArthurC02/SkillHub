---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L254-262"
    excerpt_sha256: "sha256:b0d6d3a34ace3aa0d3a099f355ed83da0e955f044dc51599f8d8f088f4ed1c1a"
  - resource: "apps/platform/internal/trial/improvement/suggest_store_test.go#L40-66"
    excerpt_sha256: "sha256:cc73bb66b0ceba6f8a592c4371428aaa668b34c26d71026d79d8b3fc1f4cde87"
---
Passed criteria are left out of the suggestion digest, so their evidence cannot be cited for an improvement.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A passed criterion's excerpt is not in the digest.
