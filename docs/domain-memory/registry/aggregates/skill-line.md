---
type: Aggregate
status: stable
root: Skill Line
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:32:47.518774Z"
sources:
  - resource: "db/migrations/0151_candidate_evaluation.sql#L1-14"
    excerpt_sha256: "sha256:3076ed5f889c6886b350200f2c7a7e8f275ab05237b3112729b3a21a848d6b4c"
  - resource: "db/migrations/0151_candidate_evaluation.sql#L16-36"
    excerpt_sha256: "sha256:9c454ba93960d791b3a3b7c0b7d44a4133f0dbf3e550e475e38761ae0c358dea"
  - resource: "db/migrations/0151_candidate_evaluation.sql#L49-52"
    excerpt_sha256: "sha256:7c0c917e2c93318b2dd7c99a9b622db5b2b382b451c8711f4dc95bb708227d61"
  - resource: "apps/platform/internal/skill/library/skill_root.go#L362-377"
    excerpt_sha256: "sha256:12dd2195eab1c66bc2742ac9bbedaa3f025c7606aa9927b40a05dae3134ef2c4"
  - resource: "apps/platform/internal/skill/library/skill_root.go#L379-396"
    excerpt_sha256: "sha256:dd32260327940f219dd784801b7c578151c68d0c2e9768f1783a34a20b91bdb6"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L514-527"
    excerpt_sha256: "sha256:b203735c35d40eca6ed1f485cc57f472deca446193dbdfeb6c243ac92ff44f4f"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L529-573"
    excerpt_sha256: "sha256:7faa748ef6e24e0eaceb54158037d7b518412949703064c738e9ae9f6a72db84"
---
# Relations
- Context: [registry](/contexts/registry)

# Invariants
- Every version in skill_versions takes its Skill's next version number, held or not, and is never rewritten.
- A Skill's line, the view skill_line_versions, is its versions that were not held for evaluation plus the held ones with a row in skill_version_promotions; the Skill's newest, latest and previous versions are read only from the line.
- A row in skill_version_promotions is written once, only for a held version of the same workspace, and is never updated or deleted, so a promoted version stays in the line.
