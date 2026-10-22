---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L66-68"
    excerpt_sha256: "sha256:0b13ab9758c7a08fce6a0d1ca2a2416e0de5871e2b4d5810ab02a5c379831315"
---
credit_entries keeps trigger credit_entries_immutable: CREATE TRIGGER credit_entries_immutable BEFORE UPDATE OR DELETE ON credit_entries FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [credit](/contexts/credit)
