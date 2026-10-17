---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:45:46.007817Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L123-128"
    excerpt_sha256: "sha256:50d5cef3b7a91903ae5428e2a722f861f3e01bd756591782ee0f2541eed44086"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L175-214"
    excerpt_sha256: "sha256:e8af8b0f200535df46a9c978e734a7f0de0d6fa35668e2dedfb58ab3c7ce26d2"
---
An event's time must fall between a few minutes in the future and the token's lifetime in the past; epoch or far-future timestamps are refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event timestamped four years ahead is refused.
