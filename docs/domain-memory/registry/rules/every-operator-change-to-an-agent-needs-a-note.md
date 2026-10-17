---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/handler.go#L206-217"
    excerpt_sha256: "sha256:ba7cdcc4692dae9bb9f57d95b7de62dc7a46856cb2b5c39d554b558831539d56"
---
Enabling or disabling an agent, changing its spend cap or its brake is refused with 400 when the note is blank.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A whitespace-only note answers 400 and the agent stays disabled.
