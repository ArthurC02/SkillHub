---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L210-213"
    excerpt_sha256: "sha256:58f0db9a7890d85a5f52681fb23546ab26392416dc1dfe38b18bd70ebaf08e2f"
---
A takedown with an empty or whitespace-only reason is refused and the Skill stays up.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A takedown with reason ' \t\n' is refused as takedown_reason_missing.
