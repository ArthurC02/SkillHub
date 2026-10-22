---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0105_run_snapshots.sql#L24-25"
    excerpt_sha256: "sha256:ac87fdaec16fc3cdb1f76f5c67a9ae94679f9fed7c950427bf115ceb97592144"
---
run_snapshots keeps trigger run_snapshots_immutable: CREATE TRIGGER run_snapshots_immutable BEFORE DELETE OR UPDATE ON run_snapshots FOR EACH ROW EXECUTE FUNCTION enforce_immutable('runtime_snapshot')

# Relations
- Contexts: [run](/contexts/run)
