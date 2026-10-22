---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0071_suggestion_version_provenance.sql#L4-5"
    excerpt_sha256: "sha256:58957f0147f7edf51bd5a14d30d9a2dc1a0a3b5a28b368610ad6d29da3c627c8"
---
skill_versions keeps unique skill_versions_id_workspace_key: CONSTRAINT skill_versions_id_workspace_key UNIQUE (id, workspace_id)

# Relations
- Contexts: [registry](/contexts/registry)
