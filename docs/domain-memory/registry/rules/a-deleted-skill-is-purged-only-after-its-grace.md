---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L134-138"
    excerpt_sha256: "sha256:94b81098b7db0f4f1b06c0ff40911850aa92d750e48f0571491c6a171639178c"
  - resource: "db/queries/governance.sql#L121-127"
    excerpt_sha256: "sha256:d76db6e6f9cbc4624c16d86dcff2be1eb0916fff993ffe611167d3a47fc9692d"
  - resource: "db/queries/governance.sql#L133-138"
    excerpt_sha256: "sha256:4c880ef90cd0672748bc4fab8ee070cf75b2fef7423de28e183729d11ee1655a"
---
A deletion sweep purges a Skill and its versions only once the Skill's deletion is older than the grace period; a sweep given a non-positive grace is refused and purges nothing.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A Skill deleted a moment ago survives a 30-day sweep and a sweep with zero grace is refused; one deleted 31 days ago with nothing depending on it is purged.
