---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L206-208"
    excerpt_sha256: "sha256:760da291190a02d8a3c6edb6197ebfbbea980cf25a0f4ec77a024867e1654200"
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L322-323"
    excerpt_sha256: "sha256:a77829a080bc3e4a4b719c99e181bf7e4a4a4c3c2b14d71837379169f7ae8c59"
  - resource: "apps/platform/internal/skill/publishing/exposure_test.go#L94-121"
    excerpt_sha256: "sha256:04ac41455b07575b937d2cfa8f8a85a5a2094b5ce4952b69bf85f164c27a8ffe"
---
A publication is exposed in the Catalog only when its review is approved, its Skill is available with redistribution allowed, and its search snapshot is listable, of the released version and with the digest the reviewer approved.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- An approved Skill whose content its creator supplied is not exposed.
