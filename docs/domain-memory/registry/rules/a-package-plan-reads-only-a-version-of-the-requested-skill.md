---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:12:31.935298Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L285-305"
    excerpt_sha256: "sha256:f195893da12e0c3fdd89965b17be5a9e2204628a3ba80737461dfa8b5c0d3f38"
  - resource: "apps/platform/internal/skill/delivery/plan_subject_characterization_test.go#L17-60"
    excerpt_sha256: "sha256:066b3f1bd8f40a9ffa0f3bf158f51c9b55d0d59ae9d38c8ad230cde015498b9a"
---
Planning a package refuses a version that belongs to another Skill, so one Skill's gates are never applied to another Skill's files.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Asking for Skill A with a version of Skill B is not found.
