---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L45-46"
    excerpt_sha256: "sha256:65185826a04f72262dbd3432578c30e394e9a5c70259bbd10d84df4e9d66a82e"
---
credit_entries keeps check credit_entries.ref_type: ref_type text CHECK (ref_type IS NULL OR ref_type IN ( 'creation_session', 'run', 'skill_version', 'operator_grant'))

# Relations
- Contexts: [credit](/contexts/credit)
