---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L141-142"
    excerpt_sha256: "sha256:bf7fac524bee552838535498210d509d3edfaded68712a57c6b93c8246c0eb9d"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L175-214"
    excerpt_sha256: "sha256:e8af8b0f200535df46a9c978e734a7f0de0d6fa35668e2dedfb58ab3c7ce26d2"
---
An event whose status is not one of the known outcomes is refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event with status 'exploded' is refused as invalid.
