---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/detail.go#L511-521"
    excerpt_sha256: "sha256:650151c16e16c25ddb3d0cc17117a9f81acd70b4dcf401f783e0de22fedd97ed"
  - resource: "apps/platform/internal/skill/discovery/detail_test.go#L74-88"
    excerpt_sha256: "sha256:5f0383a8472769fb8731f700e8ffbcbb05450cf992ddd7d583a60e8a4ccd4ea4"
---
A Skill's detail carries an access restriction only while one is in effect; a blank restriction is none, and an unrecognised reason still restricts with the default note.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- access_restriction license-review shows the licence-review note; a blank value shows no restriction.
