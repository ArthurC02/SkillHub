---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L108-111"
    excerpt_sha256: "sha256:d66233cda994e8a27f9265c9e40806c7ad7eb7ca767712fcc51a39924831a8e3"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L175-214"
    excerpt_sha256: "sha256:e8af8b0f200535df46a9c978e734a7f0de0d6fa35668e2dedfb58ab3c7ce26d2"
---
An event's attempt must be from 1 to 100000; a larger attempt is refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event with attempt 100001 is refused as invalid.
