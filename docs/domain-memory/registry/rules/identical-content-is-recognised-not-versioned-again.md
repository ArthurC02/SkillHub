---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L465-471"
    excerpt_sha256: "sha256:50c5524a8523ab9829c100da0922b8081e531304155e7c4c1c92affb1fb16dd2"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L470-526"
    excerpt_sha256: "sha256:54d0b65ca0cf0d1d222e01b4d2110a758fbc6439e45e37d80a9963a5a945d5f8"
---
Saving content whose hash a version of the same Skill already holds returns that version as a duplicate and writes no new version.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Restoring the text of an earlier version returns that version's id, marked duplicate.
