---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/runs.go#L51-60"
    excerpt_sha256: "sha256:8ade248d11061b21cdc5dd7b4f7a6bb6f8be2d16afa16184ca64f5477d1e9ec8"
---
A run halted while the brake is engaged stops with the brake's reason even when the agent is also disabled.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A braked and disabled agent's run stops saying the agent brake is engaged.
