---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0151_candidate_evaluation.sql#L49-52"
    excerpt_sha256: "sha256:7c0c917e2c93318b2dd7c99a9b622db5b2b382b451c8711f4dc95bb708227d61"
  - resource: "apps/platform/internal/skill/library/skill_root.go#L362-377"
    excerpt_sha256: "sha256:12dd2195eab1c66bc2742ac9bbedaa3f025c7606aa9927b40a05dae3134ef2c4"
  - resource: "apps/platform/internal/skill/library/skill_root.go#L398-403"
    excerpt_sha256: "sha256:5cecfbdc795b820e04d8710839246ad60fc1b0c9553d49e9195a5cb25c88089d"
  - resource: "apps/platform/internal/skill/admission/service.go#L432-435"
    excerpt_sha256: "sha256:6ec240c233093ce5e7c542614015c68228fe756c0ab2d8fe2280f0f077f1d4a7"
  - resource: "apps/platform/internal/skill/admission/service.go#L537-539"
    excerpt_sha256: "sha256:d95c00f3a76a2176bc10f6bdfa8ec1618974a2e71caba70873313d2980d6c9ed"
  - resource: "apps/platform/internal/skill/publishing/service.go#L319-322"
    excerpt_sha256: "sha256:e04ba97296ec5cf9d6b24ebce243c7114dab6545683f6e0c9268b8c6f5245fbc"
---
A version built only to evaluate a candidate of an improvement round is held: it takes its Skill's next version number and runs by its own id, but it is outside the Skill's line, so until it is promoted it is never the Skill's newest version, never describes the Skill, is never released, and is neither enriched nor written to the search index; every query that orders versions to find a newest, latest or previous one reads the line.

# Relations
- Contexts: [registry](/contexts/registry)
- Contexts: [ingest](/contexts/ingest)
- Contexts: [publishing](/contexts/publishing)

# Examples
- A Skill at v2 that holds two candidates as v3 and v4 still lists v2 as its newest, and a round started from v3's batch is refused with 409.
- Publishing a held v3 answers 422 held_for_evaluation.
- Holding two candidates makes no enrichment call and leaves the Skill's search document untouched.
