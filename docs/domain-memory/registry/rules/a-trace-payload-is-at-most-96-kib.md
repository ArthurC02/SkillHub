---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L137-138"
    excerpt_sha256: "sha256:4752673520f87440b59698125fd5e529da315fc4aca58ca12c6e71e82f80f398"
---
A trace event whose payload exceeds 96 KiB is refused as invalid; exactly 96 KiB is accepted.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A payload of 98305 bytes is refused.
