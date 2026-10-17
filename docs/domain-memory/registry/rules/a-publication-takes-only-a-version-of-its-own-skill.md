---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L299-311"
    excerpt_sha256: "sha256:cdd494d55f4fda162e5097e214ba6a75457cc5aeab8e5f940f21fbcffb51df6f"
---
Publishing a Skill with a version of another Skill is answered not found, so one Skill's gates never pass another Skill's files.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Publishing Skill A with Skill B's version answers 404.
