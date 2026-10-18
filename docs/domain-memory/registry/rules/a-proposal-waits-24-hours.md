---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L32-32"
    excerpt_sha256: "sha256:9a3652e68c3f49e6fd6bc4d91421f59c6ed6cf80ffce5f3fb1e6469e60af311f"
  - resource: "apps/platform/internal/product/operations/proposals.go#L102-106"
    excerpt_sha256: "sha256:f12ec932990687296f5b9ee4f1b9299b7e84412767fc7726dc8a34706c127e88"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L126-149"
    excerpt_sha256: "sha256:170cfb346bb7da5da662b9c6233527d92a54076a2a40b22fefc2c52b5631440b"
---
A proposed action waits 24 hours for a decision and then lapses.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A proposal made now expires 24 hours later.
