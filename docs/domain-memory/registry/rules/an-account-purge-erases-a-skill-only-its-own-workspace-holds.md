---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T05:48:39.999420Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/governance_integration_test.go#L727-789"
    excerpt_sha256: "sha256:1a12a02dc9ccd89d0b758f4ceb14911c4c48488ce847174bc0ec91813b3591c2"
  - resource: "apps/platform/internal/skill/library/purge.go#L41-87"
    excerpt_sha256: "sha256:d801eb8bd393e660cfe7fc39fe1515c442f9f3d066d0480226adce6bd7afdf0a"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/governance_integration_test.go#L980-998"
    excerpt_sha256: "sha256:51b7b316b3d25c27fab824e84431149643a0c04b865ed61813ea35b9b64ab0b2"
---
An account purge erases the content of a skill that only the workspace's own Runs or frozen test cases hold: every version loses its manifest and import source, its package object goes to collection, the skill loses its summary and search entry, and the version rows stay so those Runs still point at them. A skill another workspace forked, bundled or downloaded keeps its content. Every skill the purge keeps is marked deleted and leaves search, so the deletion sweep removes it once nothing holds it any more.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Erin's private skill used by one of her Runs: after her purge the Run and a version row with an empty manifest remain, the import source and search entry are gone, and collection removes the package; Alice's skill that Bob forked keeps its content.
- Alice's skill that Bob forked is marked deleted by her purge, so once Bob's fork is gone the sweep can remove it.
