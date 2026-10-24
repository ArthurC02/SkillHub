---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/admission/generate.go#L203-206"
    excerpt_sha256: "sha256:baca19f906363d2d3f9a5f102e77f1a332eb9b6b632b3da8de804967371a0f30"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/generate_integration_test.go#L360-374"
    excerpt_sha256: "sha256:51ac0a2ef78c56ec3af0b5bfcbc722850a7c639c4273fb6e80a2133e1e989e6c"
---
The catalogue workspace never generates skills; a generation request in it is refused before anything else is checked.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A generation asked of the catalogue workspace is refused as not for the catalogue.
