---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T14:10:36.977281Z"
sources:
  - resource: "apps/platform/internal/skill/admission/generate.go#L213-215"
    excerpt_sha256: "sha256:c0bdf3a9817a6b117d3afccc7e4662e41d9b8aa8b9c51c037e8a1d7b6953ba43"
  - resource: "apps/platform/internal/skill/admission/generate_test.go#L602-612"
    excerpt_sha256: "sha256:0a15b2b1c6a79ebddd0e1565369089bc82b1065e24dabb466a78ac0d3f84f336"
---
A generation names at most three reference skills; three are accepted and four are refused before the gateway.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Three references reach the gateway; four are refused.
