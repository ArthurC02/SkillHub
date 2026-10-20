---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T07:03:29.672830Z"
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L41-71"
    excerpt_sha256: "sha256:866738e72532fe15739b6f8ebfd90b1e732d6b35e287fb26c292dcec4bb672a9"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/governance_integration_test.go#L727-789"
    excerpt_sha256: "sha256:1a12a02dc9ccd89d0b758f4ceb14911c4c48488ce847174bc0ec91813b3591c2"
---
An account purge erases the content of a skill that only the workspace's own Runs or frozen test cases hold: every version loses its manifest and import source, its package object goes to collection, the skill loses its summary and search entry, and the version rows stay so those Runs still point at them. A skill another workspace forked, bundled or downloaded keeps its content.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Erin's private skill used by one of her Runs: after her purge the Run and a version row with an empty manifest remain, the import source and search entry are gone, and collection removes the package; Alice's skill that Bob forked keeps its content.
