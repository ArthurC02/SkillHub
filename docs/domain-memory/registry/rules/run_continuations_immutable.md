---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L18-20"
    excerpt_sha256: "sha256:31d814a0414f5637b84b2aeb27b5c85567b8e8d70f46668b52a51d248f73a70e"
---
run_continuations keeps trigger run_continuations_immutable: CREATE TRIGGER run_continuations_immutable BEFORE UPDATE ON run_continuations FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [run](/contexts/run)
