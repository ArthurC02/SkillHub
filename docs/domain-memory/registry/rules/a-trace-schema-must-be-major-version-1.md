---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L112-113"
    excerpt_sha256: "sha256:8a5124ccad973b1f97b4135664a5c3244b0a91715972c7a388c5d5669661e98b"
  - resource: "apps/platform/internal/trial/evidence/event.go#L167-169"
    excerpt_sha256: "sha256:51b83b424fa0df583f8b58813b285b970c0fab9935dd3cd5e61809812dd9d376"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L175-214"
    excerpt_sha256: "sha256:e8af8b0f200535df46a9c978e734a7f0de0d6fa35668e2dedfb58ab3c7ce26d2"
---
An event is accepted only under a 1.x schema version; another major version is refused while additive minor versions are accepted.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- schema_version 2.0 is refused; 1.7 is accepted.
