---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:52:30.207485Z"
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L233-251"
    excerpt_sha256: "sha256:b6ab54bc9c24068546ecdb5eddc307b944afe41fbecae8f67cbd045345872845"
  - resource: "apps/platform/internal/skill/library/purge_test.go#L53-95"
    excerpt_sha256: "sha256:7ce143f0027e5271d25fbf28cb05b158f370d3f6c67c2755b12858ac6f1384a8"
---
A purge keeps a source skill while any fork of it stays, transitively down fork chains.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A deleted skill forked by another user's live skill survives the sweep.
