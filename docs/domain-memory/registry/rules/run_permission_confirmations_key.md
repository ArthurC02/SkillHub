---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0020_run_permission_confirmations.sql#L11-12"
    excerpt_sha256: "sha256:1953792eba1458469940d53449f9a6c34e62064bec76727930968712159df537"
---
run_permission_confirmations keeps unique-index run_permission_confirmations_key: CREATE UNIQUE INDEX run_permission_confirmations_key ON run_permission_confirmations (workspace_id, skill_version_id, test_case_id, summary_hash)

# Relations
- Contexts: [run](/contexts/run)
