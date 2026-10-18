---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/run_fetch_commands.go#L71-73"
    excerpt_sha256: "sha256:f154f9f48c415d929c412cb87709ae595a28f6ae6d2192af54fc8e01bd7ed35a"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L512-524"
    excerpt_sha256: "sha256:81824bf8ad0a6f5227c7865ffb9a6c56fbee07e55468b3745ed76ec98d75d3e5"
---
Raising a creation budget is refused unless the new amount is finite, strictly above the current budget and within the ceiling.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- Raising 0.5 to 0.5 is refused.
