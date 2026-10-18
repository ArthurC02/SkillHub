---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/config.go#L40-42"
    excerpt_sha256: "sha256:7ad5af62915b8c6cb17858d4ccedb8a7a4b68a0b9cdee7664e3a9d3e4a9d9058"
  - resource: "apps/platform/internal/creator/credit/config_test.go#L16-23"
    excerpt_sha256: "sha256:d636e93e678cbf4c052fae6d4da346bc2506af8b90b2c3de4b8c516051ca30e9"
---
Configuration refuses a debt floor above zero, which would demand a surplus before any spend.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- A floor of 1 is refused; 0 and -50 are accepted.
