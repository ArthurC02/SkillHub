---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/access.go#L32-32"
    excerpt_sha256: "sha256:bf1b3689ee510e485e0b1c7117ae8628593ecf3a1bef01760ef6bb009a97a2aa"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/download_integration_test.go#L758-782"
    excerpt_sha256: "sha256:ddcbadf30a0e71f041c1a373eddac84b21c3942f7a2d400bd0f6455d09fa1188"
---
A Skill without its own access restriction carries the nearest restricted ancestor's along its fork lineage, so a hold placed after a fork was made still holds the fork, without the fork's row being written.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A hold on the grandparent withholds the grandchild fork's download; lifting it restores the download.
