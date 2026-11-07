---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:32:47.518774Z"
sources:
  - resource: "db/migrations/0151_candidate_evaluation.sql#L16-32"
    excerpt_sha256: "sha256:edbda295ccde891102dcd00c650e45a8af90d61943bced7a5c1d5dd0c02355cb"
  - resource: "apps/platform/internal/skill/admission/promote.go#L20-62"
    excerpt_sha256: "sha256:4b702d940e7c3f8319e961270ed01dc41847f97825e81004d4fe43816a84c303"
  - resource: "apps/platform/internal/skill/library/skill_root.go#L379-396"
    excerpt_sha256: "sha256:dd32260327940f219dd784801b7c578151c68d0c2e9768f1783a34a20b91bdb6"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L529-573"
    excerpt_sha256: "sha256:7faa748ef6e24e0eaceb54158037d7b518412949703064c738e9ae9f6a72db84"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/candidate_evaluation_integration_test.go#L147-186"
    excerpt_sha256: "sha256:fcaa903701a106f86a25f1356f7ecdd0d567e6969e25a190f84ac8fe1c090ad3"
---
Adopting a candidate promotes its held version into the Skill's line only while the line's newest version is still the version the round read; promoting it again changes nothing, a version that was never held cannot be promoted, and the promoted version is enriched, indexed and describes the Skill once.

# Relations
- Contexts: [registry](/contexts/registry)
- Contexts: [ingest](/contexts/ingest)

# Examples
- A held v3 built from v2 is promoted while v2 is the newest and becomes the newest; a second adoption of it records nothing new.
- After a v4 was uploaded the adoption is refused because the line moved on, and v3 stays held.
- Adopting the same candidate twice makes exactly one enrichment call and one debit.
