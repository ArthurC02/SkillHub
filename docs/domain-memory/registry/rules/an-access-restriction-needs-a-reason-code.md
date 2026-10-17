---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L218-221"
    excerpt_sha256: "sha256:1a892d5d9eba5f0d6c19bf1d3296453cba241953f0d3ddf6c96b47d82ec26ff4"
---
An access restriction must carry a non-blank reason code; a blank one is refused and the Skill's restriction is unchanged. A restriction is lifted with none, not blank.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Setting the restriction to '  ' is refused and the column stays empty.
