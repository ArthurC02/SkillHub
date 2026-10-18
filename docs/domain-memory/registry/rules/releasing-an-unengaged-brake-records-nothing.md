---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/agents.go#L203-214"
    excerpt_sha256: "sha256:f483f5c5d2072f5b1177577b0307267473ff2dff0c43b63f849649bbc402884a"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agents_integration_test.go#L214-258"
    excerpt_sha256: "sha256:87547b2a40c77885bd8df33135e62fb7bcc70b9bb558c1bc463acd36bc926a24"
---
Releasing the brake when none is engaged succeeds without recording a brake-released audit event.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A second release adds no brake_released event.
