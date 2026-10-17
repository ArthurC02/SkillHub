---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:43:00.552336Z"
sources:
  - resource: "apps/platform/internal/skill/library/diff.go#L45-47"
    excerpt_sha256: "sha256:b7bb4c3fac09517d946964e992970e4c4a26d466362481868b2c1f215b0b73de"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/skill_diff_integration_test.go#L78-96"
    excerpt_sha256: "sha256:c07a1d9c09511fd1dbe3b4446ba061f061fee8682b4b7c8f5a643f257a9785a5"
---
While a Skill is under an access restriction, its version diffs reveal no content; the request answers 403.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- The diff of a Skill under license-review answers 403 with no files.
