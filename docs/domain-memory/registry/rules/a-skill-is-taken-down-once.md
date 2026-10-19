---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:32:50.132592Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L206-209"
    excerpt_sha256: "sha256:3cbf71fa47d03be6f2fd425040d690723372ce560beac77080e4b4c8a2b4af2a"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L26-43"
    excerpt_sha256: "sha256:e19237875319083945f7841bf7a8794d596cde976e85c19264f31b6ace865687"
---
A Skill already taken down cannot be taken down again; the second attempt is refused and the Skill stays down.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A second report against a taken-down Skill is refused as already_taken_down.
