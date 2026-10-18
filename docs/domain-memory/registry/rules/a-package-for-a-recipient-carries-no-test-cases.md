---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L338-341"
    excerpt_sha256: "sha256:07eac29f16383f2d1e877412053be34c075096d072763237963bfc6cfda6566c"
  - resource: "apps/platform/internal/skill/delivery/source_rules_test.go#L94-104"
    excerpt_sha256: "sha256:77e8cf7685535968a2b9d7b4e6b0e87fb8e6691823f12fb150ab7db521bcbc69"
---
A package built for a recipient never selects test cases, whatever the include flag says.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A recipient package asked to include test cases holds none.
