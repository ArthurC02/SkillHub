---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T00:09:09.697460Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L208-216"
    excerpt_sha256: "sha256:3a96ece110e80bc90d85b889257a4ac3e375c87a0b13c63b8e397a7d78e8ff6c"
  - resource: "apps/platform/internal/skill/publishing/exposure_test.go#L94-127"
    excerpt_sha256: "sha256:53be478221804a44b69649595cbced6b3b5a8ae3d201afea79b3d374a4b9691a"
---
A publication is exposed in the Catalog only when its review is approved, its Skill is available and cleared for the Catalog, and its search snapshot is listable, of the released version and with the digest the reviewer approved. A Skill is cleared for the Catalog when its redistribution is allowed, or when it was generated and its latest release declares a licence.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- An approved Skill whose content its creator supplied is not exposed, even with a licence on its release; an approved generated Skill is exposed once its latest release declares MIT.
