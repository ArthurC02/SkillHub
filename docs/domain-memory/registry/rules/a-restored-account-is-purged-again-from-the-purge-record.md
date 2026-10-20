---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/purge.go#L223-248"
    excerpt_sha256: "sha256:1a1aea9203032b0a12db3b953372860f8c4a48813b3b318e45cdfda2f9ac243f"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/account_purge_characterization_test.go#L270-314"
    excerpt_sha256: "sha256:b4198453dced69c1b88459637380e88e6fd87a70cc3032efac871303e43772a8"
---
Replaying the purge record registers an erasure for, and purges at once, every recorded account the database holds alive; it leaves accounts already purged and accounts the database does not hold alone, and fails naming any record it cannot read or any account it could not purge.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- After restoring a backup taken before Alice's purge, replay purges Alice again and only Alice; a second replay purges no one; a record named not-an-account makes the replay fail naming it.
