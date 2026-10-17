---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/name.go#L23-34"
    excerpt_sha256: "sha256:3dc7a4bf958ec80d38e64f35a58362ff973860d7ee32f4e4859a7d2bb611aa9d"
  - resource: "apps/platform/internal/skill/publishing/publication_test.go#L12-41"
    excerpt_sha256: "sha256:ec494c62d0125a668439124c83197ca3b2863a05aa2ce1036408020f0addb263"
---
A publisher name follows the Skill name shape and must not equal a reserved word once its hyphens are removed; a publication name follows only the shape.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- s-k-i-l-l-h-u-b is refused as a publisher name.
