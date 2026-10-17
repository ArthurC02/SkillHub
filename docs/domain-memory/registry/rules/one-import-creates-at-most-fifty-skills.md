---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:35:41.095408Z"
sources:
  - resource: "apps/platform/internal/skill/admission/source.go#L76-78"
    excerpt_sha256: "sha256:12a8ec67d431c4fd07d815729db475426ea4f1af333175b51e3b31eb8b92ade4"
  - resource: "apps/platform/internal/skill/admission/source_test.go#L180-196"
    excerpt_sha256: "sha256:03e39b80d7e226ef5bffcf92d564182f9049ca1a13ac3de3e16a939ebd2821d9"
---
One import may create at most MaxSkillsPerImport (50) Skills; a source that would create more is refused as a whole, naming the count it would create and the ceiling. Skills refused for other reasons do not count.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A plugin with 51 valid Skills is refused; one with 50 imports.
