---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L285-305"
    excerpt_sha256: "sha256:f195893da12e0c3fdd89965b17be5a9e2204628a3ba80737461dfa8b5c0d3f38"
---
Planning a package refuses a version that belongs to another Skill, so one Skill's gates are never applied to another Skill's files.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Asking for Skill A with a version of Skill B is not found.
