---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/diff.go#L47-53"
    excerpt_sha256: "sha256:999651f074567595d3fce5c551169cbe080d3afb20af1379ab1ca2469286f027"
  - resource: "apps/platform/internal/skill/library/content_guards_test.go#L46-74"
    excerpt_sha256: "sha256:010372b1c7653f7d51585c1d038354f8a04da190d14a723c4663544c7aa11ab9"
---
A version diff finds nothing when a named version belongs to another skill, even in the same Workspace.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Diffing skill A against a version of skill B answers not found and reads no package.
