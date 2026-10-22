---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0105_run_snapshots.sql#L26-27"
    excerpt_sha256: "sha256:4e0dda9f66954056db19394979a3198ef069c0e8738070c830144f3c4d656004"
---
run_snapshots keeps trigger run_snapshots_open_until_finished: CREATE TRIGGER run_snapshots_open_until_finished BEFORE UPDATE ON run_snapshots FOR EACH ROW EXECUTE FUNCTION enforce_run_snapshot_open()

# Relations
- Contexts: [run](/contexts/run)
