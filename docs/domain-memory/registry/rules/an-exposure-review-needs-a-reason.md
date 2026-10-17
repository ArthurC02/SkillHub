---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L401-407"
    excerpt_sha256: "sha256:d292880236791f5fbdaad4c9c9993cca3fe6691ae4ff858cfaf2c4ff071b031e"
---
An exposure review is refused before any lookup unless its decision is approved or revoked and it carries a non-blank reason.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- An approval whose reason is only spaces answers 422 reason_missing.
