---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T14:46:47.256731Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/intent.go#L124-134"
    excerpt_sha256: "sha256:c7015d518852e5841ddf0114afb760357b54f77c0f58229e4ea2dbec2c026681"
  - resource: "apps/platform/internal/skill/discovery/intent_test.go#L62-84"
    excerpt_sha256: "sha256:87f55842163246cf145bee4d95a36bb7a68a96254521e413fe3fcf02192c9740"
---
A search interpretation holds at most 8 keywords, each non-blank and at most 128 characters; exactly 8 keywords of 128 characters are accepted.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- Eight keywords of 128 characters are accepted; a ninth keyword or a 129-character one is refused.
