---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L123-128"
    excerpt_sha256: "sha256:50d5cef3b7a91903ae5428e2a722f861f3e01bd756591782ee0f2541eed44086"
---
An event's time must fall between a few minutes in the future and the token's lifetime in the past; epoch or far-future timestamps are refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event timestamped four years ahead is refused.
