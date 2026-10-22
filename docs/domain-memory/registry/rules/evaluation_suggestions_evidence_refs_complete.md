---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L75-79"
    excerpt_sha256: "sha256:8a090d66e37811c23cc9c796649d2636068fef48fc1e98b95df003ddf9c91d3f"
---
evaluation_suggestions keeps check evaluation_suggestions_evidence_refs_complete: CONSTRAINT evaluation_suggestions_evidence_refs_complete CHECK ( NOT jsonb_path_exists( evidence, '$[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))') )

# Relations
- Contexts: [eval](/contexts/eval)
