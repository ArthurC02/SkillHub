---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:59:29.446208Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:54.156118Z"
sources:
  - resource: "apps/platform/internal/product/operations/runs.go#L38-40"
    excerpt_sha256: "sha256:961a6abea3226cb0a412d101300da6088277cbff82ee17c0bcb8d057e26b5ce5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agents_integration_test.go#L168-212"
    excerpt_sha256: "sha256:c05a77d6b18c1503df9e71320b54eee6295bc8bf5bfb9a47d0c4de0d18086a6f"
---
A run that has finished cannot take another step.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- Stepping a run after it finished answers run finished.
