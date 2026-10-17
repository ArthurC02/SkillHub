---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:43:00.552336Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L210-213"
    excerpt_sha256: "sha256:58f0db9a7890d85a5f52681fb23546ab26392416dc1dfe38b18bd70ebaf08e2f"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L44-60"
    excerpt_sha256: "sha256:12dbd73b03a83bcc1969d75038191d923ee1e0d01d4b2169034616daf7868236"
---
A takedown with an empty or whitespace-only reason is refused and the Skill stays up.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A takedown with reason ' \t\n' is refused as takedown_reason_missing.
