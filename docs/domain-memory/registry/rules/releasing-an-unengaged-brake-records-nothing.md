---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-12T09:34:36.964247Z"
sources:
  - resource: "apps/platform/internal/product/operations/agents.go#L204-215"
    excerpt_sha256: "sha256:f483f5c5d2072f5b1177577b0307267473ff2dff0c43b63f849649bbc402884a"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agents_integration_test.go#L209-253"
    excerpt_sha256: "sha256:35fbfb8dbeeada26d3ad30f92dfa2e661983dc0dd434de25a7d1f3e77471bbb5"
---
Releasing the brake when none is engaged succeeds without recording a brake-released audit event.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A second release adds no brake_released event.
