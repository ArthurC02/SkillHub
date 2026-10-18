---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L112-115"
    excerpt_sha256: "sha256:818d9d3ec796397abdc662a218e1b4d3264f4f5dd846a798dff95590f8392b20"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L175-214"
    excerpt_sha256: "sha256:e8af8b0f200535df46a9c978e734a7f0de0d6fa35668e2dedfb58ab3c7ce26d2"
---
An event's seq must be from 1 to 100000; a larger seq is refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event with seq 100001 is refused as invalid.
