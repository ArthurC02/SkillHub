---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:32:50.132592Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L210-213"
    excerpt_sha256: "sha256:58f0db9a7890d85a5f52681fb23546ab26392416dc1dfe38b18bd70ebaf08e2f"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L78-94"
    excerpt_sha256: "sha256:1728840bd75ad3a4d5f255ac9870184938de6da3396c9ccdad517137fa6d0e97"
---
A takedown with an empty or whitespace-only reason is refused and the Skill stays up.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A takedown with reason ' \t\n' is refused as takedown_reason_missing.
