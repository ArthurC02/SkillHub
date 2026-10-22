---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L150-151"
    excerpt_sha256: "sha256:52b466a2f9e4765941f228bd93bdc890a36fd6e5e1f86502350f20e0b2dd579f"
---
artifacts keeps check artifacts.scan_status: scan_status text NOT NULL DEFAULT 'quarantined' CHECK (scan_status IN ('quarantined', 'available', 'rejected'))

# Relations
- Contexts: [run](/contexts/run)
