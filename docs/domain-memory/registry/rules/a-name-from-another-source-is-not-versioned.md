---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:35:41.095408Z"
sources:
  - resource: "apps/platform/internal/skill/admission/source.go#L250-282"
    excerpt_sha256: "sha256:e97463c4ca7d531e92a5e5c89c4b669aa40a8c601ef87192fda95caafb2c331c"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/import_source_integration_test.go#L353-390"
    excerpt_sha256: "sha256:396e2d2e70bb25822cd556bbd4a109233b83ac34e3a32fc2ef4290a521b3261d"
---
A Skill arriving from a plugin or URL is refused as name_held_by_another_source when the same-named Skill's latest version came from a different plugin or URL; the rest of the import proceeds. Uploads without an origin and generated sources are exempt.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Plugin team-kit's code-review is refused when code-review came from plugin desk-tools; team-kit's other Skills import.
