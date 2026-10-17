---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L134-138"
    excerpt_sha256: "sha256:94b81098b7db0f4f1b06c0ff40911850aa92d750e48f0571491c6a171639178c"
  - resource: "apps/platform/internal/skill/library/purge.go#L151-153"
    excerpt_sha256: "sha256:2dc18e68765cf5803c20111f22fb4ca3062ec4c872705be772a2b6b12154e522"
  - resource: "apps/platform/internal/skill/library/purge.go#L168-168"
    excerpt_sha256: "sha256:e06f10a534d5f83f73092084520451c59a9e7c84b39d6bbad9a479a9cf77c199"
---
A deletion sweep purges a Skill and its versions only once the Skill's deletion is older than the grace period; a sweep given a non-positive grace is refused and purges nothing.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A Skill deleted a moment ago survives a 30-day sweep and a sweep with zero grace is refused; one deleted 31 days ago with nothing depending on it is purged.
