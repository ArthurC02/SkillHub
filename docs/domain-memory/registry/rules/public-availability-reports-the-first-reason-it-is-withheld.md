---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:50:56.766681Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/publication.go#L108-122"
    excerpt_sha256: "sha256:41c189c32a7a6eec1c38be2bcf2c4876e04e5f939d4ba2e28686d3228f5aa396"
  - resource: "apps/platform/internal/skill/publishing/publication_test.go#L86-109"
    excerpt_sha256: "sha256:b0a0e2f2522e70cafbf01f0c0e40dc6bf4d2cada3782f3200a8f0c1e8452a605"
---
A publication is offered only while it is published and its Skill exists, is not taken down, is not held and still passes the release gate; otherwise the first failing reason, in the order delisted, withdrawn, taken_down, held, not_redistributable, is reported.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A delisted publication whose Skill is also taken down reports delisted.
