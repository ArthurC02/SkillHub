---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:43:00.552336Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L206-209"
    excerpt_sha256: "sha256:3cbf71fa47d03be6f2fd425040d690723372ce560beac77080e4b4c8a2b4af2a"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L25-42"
    excerpt_sha256: "sha256:daea6ba494f86d15ddfb8f6d802164bc58636ae510c45d2ce75bc1c8c6d81f06"
---
A Skill already taken down cannot be taken down again; the second attempt is refused and the Skill stays down.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A second report against a taken-down Skill is refused as already_taken_down.
