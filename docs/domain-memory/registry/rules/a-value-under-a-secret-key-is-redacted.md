---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/mask.go#L150-153"
    excerpt_sha256: "sha256:260a1c4f787a2fedb35c9fd48b86cc99a30750586634755b3528a5b5d4b45024"
  - resource: "apps/platform/internal/trial/evidence/mask.go#L224-232"
    excerpt_sha256: "sha256:9cef7c5b948702a4a0c182f7ecb1eb9728531c903bb47e744afcaa8d5734c3a7"
---
Any value under a JSON key whose name denotes a secret, such as password, token, api key, authorization or cookie, is stored as [REDACTED] and its path is reported as masked.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- {"client_secret":"opaque-value-123"} is stored redacted.
