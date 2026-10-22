---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0051_unmanifested_object_cleanup_intents.sql#L25-25"
    excerpt_sha256: "sha256:680139115a6c37f951faeec549d014e9a58be2c466d0bdc5796032cc8c4897dc"
---
run_artifact_upload_intents keeps unique run_artifact_upload_intents.run_attempt_id: run_attempt_id uuid NOT NULL UNIQUE REFERENCES run_attempts(id) ON DELETE CASCADE

# Relations
- Contexts: [run](/contexts/run)
