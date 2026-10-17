---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/diff.go#L45-47"
    excerpt_sha256: "sha256:b7bb4c3fac09517d946964e992970e4c4a26d466362481868b2c1f215b0b73de"
---
While a Skill is under an access restriction, its version diffs reveal no content; the request answers 403.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- The diff of a Skill under license-review answers 403 with no files.
