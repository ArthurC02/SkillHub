---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0071_suggestion_version_provenance.sql#L1-2"
    excerpt_sha256: "sha256:13ba1e35889201fa5cd382b24129229af8b05e44d9371d1bae72c6a04c09f44c"
---
evaluation_suggestions keeps unique evaluation_suggestions_id_workspace_key: CONSTRAINT evaluation_suggestions_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [eval](/contexts/eval)
