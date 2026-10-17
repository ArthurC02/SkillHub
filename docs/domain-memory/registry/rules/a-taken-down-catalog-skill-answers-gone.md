---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:57:33.844230Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/detail.go#L566-573"
    excerpt_sha256: "sha256:9594d3f322ced510bb685a1e06966aa2e8f697846f9d426d7c57e51310210a93"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/takedown_operator_integration_test.go#L49-100"
    excerpt_sha256: "sha256:3b80f399d6d7a495b475225eb828d68fb3768664a867817dc91e5f055f063c58"
---
Opening a Catalog Skill that has been taken down answers 410 and returns nothing of it.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- An operator takes a Skill down; its detail view answers 410.
