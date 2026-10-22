---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0050_run_attempt_object_grant_expiry.sql#L1-5"
    excerpt_sha256: "sha256:01df26c4130c2f464d64f61d76220b7f1aea33dff4d4122cce9794d35f792a49"
---
run_attempts keeps check run_attempts.object_grants_state: object_grants_state text NOT NULL DEFAULT 'legacy_unknown' CHECK (object_grants_state IN ('legacy_unknown', 'unissued', 'recorded', 'closed'))

# Relations
- Contexts: [run](/contexts/run)
