---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T16:04:44.915924Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/plugin.go#L130-143"
    excerpt_sha256: "sha256:04d9a52c09edbc06af28cd24f158fe80bc7830fb8bff3de74962c956d8ea9716"
  - resource: "apps/platform/internal/skill/delivery/source_rules_test.go#L73-92"
    excerpt_sha256: "sha256:82d40426b7c06ef9cb276ca1410ae577012a997fc0babc8cbb6bf146487e78af"
---
A plugin member whose version belongs to another Skill is answered not found, so one Skill's gates never apply to another's content.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A member naming skill A with a version of skill B is not found.
