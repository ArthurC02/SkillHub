---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L408-414"
    excerpt_sha256: "sha256:d292880236791f5fbdaad4c9c9993cca3fe6691ae4ff858cfaf2c4ff071b031e"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/exposure_integration_test.go#L311-345"
    excerpt_sha256: "sha256:7b742849aada15e4ee3cdfb0bd32ac1679e59301ee34662faeaa5674f6077a7e"
---
An exposure review is refused before any lookup unless its decision is approved or revoked and it carries a non-blank reason.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- An approval whose reason is only spaces answers 422 reason_missing.
