---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L449-455"
    excerpt_sha256: "sha256:50c5524a8523ab9829c100da0922b8081e531304155e7c4c1c92affb1fb16dd2"
---
Saving content whose hash a version of the same Skill already holds returns that version as a duplicate and writes no new version.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Restoring the text of an earlier version returns that version's id, marked duplicate.
