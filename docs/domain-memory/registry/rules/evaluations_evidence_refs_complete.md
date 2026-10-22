---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0024_evaluation.sql#L25-33"
    excerpt_sha256: "sha256:7919764dadc74a3fbb8f11120c23d701abf7216a3126840c86a0d920715c9f9d"
---
evaluations keeps check evaluations_evidence_refs_complete: CONSTRAINT evaluations_evidence_refs_complete CHECK ( NOT jsonb_path_exists( criterion_results, '$[*].evidence[*] ? (!exists(@.kind) || !exists(@.available) || !exists(@.excerpt))') AND NOT jsonb_path_exists( deterministic_findings, '$[*].…

# Relations
- Contexts: [eval](/contexts/eval)
