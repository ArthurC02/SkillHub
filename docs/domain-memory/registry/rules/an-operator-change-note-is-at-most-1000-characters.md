---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/operations/handler.go#L202-213"
    excerpt_sha256: "sha256:ba7cdcc4692dae9bb9f57d95b7de62dc7a46856cb2b5c39d554b558831539d56"
  - resource: "apps/platform/internal/product/operations/note_test.go#L10-31"
    excerpt_sha256: "sha256:6f4768109243225bd2a3fe4fda3cc8b786aa1a46508919986f17d2b6c6c2c72c"
---
An operator's note on an agent change is at most the contract's cap counted in characters; one more is refused with 400.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A note of exactly the cap in Chinese characters is accepted; one more is refused.
