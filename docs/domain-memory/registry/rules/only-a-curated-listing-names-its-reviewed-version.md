---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/projection.go#L147-149"
    excerpt_sha256: "sha256:cfb9cac5eb1c1a3638fda9fe2841f13917ce1ad05973589b96b707a873e910f9"
  - resource: "apps/platform/internal/skill/discovery/projection_test.go#L46-59"
    excerpt_sha256: "sha256:573e2823728cb4cd0fa78b25379c5f74f857fbb332f6f7d3c34ccbdf256c12c6"
---
A listing records a curated version only when the Skill's tier is curated; any other tier writes none.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- Indexed-tier facts that carry a curated version id write none.
