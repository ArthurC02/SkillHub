---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L206-209"
    excerpt_sha256: "sha256:3cbf71fa47d03be6f2fd425040d690723372ce560beac77080e4b4c8a2b4af2a"
---
A Skill already taken down cannot be taken down again; the second attempt is refused and the Skill stays down.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A second report against a taken-down Skill is refused as already_taken_down.
