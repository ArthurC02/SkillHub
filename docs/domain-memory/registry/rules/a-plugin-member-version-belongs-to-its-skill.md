---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-05T16:50:27.141932Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/source_rules_test.go#L73-92"
    excerpt_sha256: "sha256:82d40426b7c06ef9cb276ca1410ae577012a997fc0babc8cbb6bf146487e78af"
  - resource: "apps/platform/internal/skill/delivery/plugin.go#L150-166"
    excerpt_sha256: "sha256:0e2620fd8e81ce9fea502611457d1ac54cd87f7faababea6a431fc2d0785b19f"
---
A plugin member whose version belongs to another Skill is answered not found, so one Skill's gates never apply to another's content.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A member naming skill A with a version of skill B is not found.
