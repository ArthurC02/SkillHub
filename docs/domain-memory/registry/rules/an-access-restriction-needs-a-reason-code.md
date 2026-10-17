---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:43:00.552336Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L218-221"
    excerpt_sha256: "sha256:1a892d5d9eba5f0d6c19bf1d3296453cba241953f0d3ddf6c96b47d82ec26ff4"
  - resource: "apps/platform/internal/skill/library/restriction_test.go#L14-47"
    excerpt_sha256: "sha256:5cd3c0bf024a0884a9c6ad362c11708518eb327818ac2442aa3818141a80d895"
---
An access restriction must carry a non-blank reason code; a blank one is refused and the Skill's restriction is unchanged. A restriction is lifted with none, not blank.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Setting the restriction to '  ' is refused and the column stays empty.
