---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/runs.go#L25-31"
    excerpt_sha256: "sha256:da9810adf7a3fcb6d44edb313528cd30c3efb4e0958c6fb0af4886ea23b99a90"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agents_integration_test.go#L168-212"
    excerpt_sha256: "sha256:c05a77d6b18c1503df9e71320b54eee6295bc8bf5bfb9a47d0c4de0d18086a6f"
---
Starting a run for a disabled, unknown or braked agent is refused as halted and records no run.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A disabled agent's start answers halted.
