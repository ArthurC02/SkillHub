---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T04:31:47.232098Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L230-241"
    excerpt_sha256: "sha256:cc0770c9e2faba761708ded728f17d925225bf36767705877080d4b7eeb11053"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L26-68"
    excerpt_sha256: "sha256:1bf6a2c8ea47c0aada434e072d458147ea3c7e9c53487be898f76749561c3799"
---
A Skill already taken down cannot be taken down again, except that an operator's takedown supersedes the owner's own: the Skill stays down, now as the operator's, and only an operator lifts it. Any other second attempt is refused and the Skill stays down.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A second report against a taken-down Skill is refused as already_taken_down.
- An operator taking down a Skill its owner already took down succeeds; the owner can no longer restore it.
