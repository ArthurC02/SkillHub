---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/handler.go#L206-217"
    excerpt_sha256: "sha256:ba7cdcc4692dae9bb9f57d95b7de62dc7a46856cb2b5c39d554b558831539d56"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agents_integration_test.go#L260-286"
    excerpt_sha256: "sha256:28067289ca49b4627080a9ce9d0b5da5ac32fe87276d8b6651b52b9c4f5945bc"
---
Enabling or disabling an agent, changing its spend cap or its brake is refused with 400 when the note is blank.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A whitespace-only note answers 400 and the agent stays disabled.
