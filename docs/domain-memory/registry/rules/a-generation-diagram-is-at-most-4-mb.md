---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T14:10:36.977281Z"
sources:
  - resource: "apps/platform/internal/skill/admission/generate.go#L207-212"
    excerpt_sha256: "sha256:8249101e926ab6300def8a271a0eba1a8f3a168a2b13db4c074a9c6027c56523"
  - resource: "apps/platform/internal/skill/admission/generate_test.go#L571-588"
    excerpt_sha256: "sha256:ad1db3c81dd8013210c4778612c57bfef603c046fd83d9fa68288786ce01e1de"
---
A diagram attached to a generation must be a non-empty image of an allowed type and at most 4,000,000 bytes; a diagram of exactly that size is accepted.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A 4,000,000-byte PNG is accepted; one byte more is refused before the gateway.
