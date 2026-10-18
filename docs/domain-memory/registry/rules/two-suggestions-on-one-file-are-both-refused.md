---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L518-534"
    excerpt_sha256: "sha256:5621af201592434ce12d84fafafe4a63300abc4d633bd0dbde5ab52624cc16b4"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L565-626"
    excerpt_sha256: "sha256:f6b83c3c2e5ccdddf06bfdad1f67e9c4d9eeb2dad40b5887c395522d562d532d"
---
When two selected suggestions target the same file, both are refused rather than the last one winning.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Two replacements of SKILL.md answer 422 listing both, and no version is built.
