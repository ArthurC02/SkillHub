---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0005_immutability.sql#L41-43"
    excerpt_sha256: "sha256:3df17efe9b6d7979cc63e93f210bc714aee96f3927e004ced5698ad91d28d8ab"
---
test_case_snapshots keeps trigger test_case_snapshots_immutable: CREATE TRIGGER test_case_snapshots_immutable BEFORE UPDATE OR DELETE ON test_case_snapshots FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [testlab](/contexts/testlab)
