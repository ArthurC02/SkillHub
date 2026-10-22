---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0060_credit_ledger.sql#L26-28"
    excerpt_sha256: "sha256:303346cd09d6230fc05bb6492675e95c17f4c5cbff0008622ccdddbd75d6b5a4"
---
cost_events keeps trigger cost_events_immutable: CREATE TRIGGER cost_events_immutable BEFORE UPDATE OR DELETE ON cost_events FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [credit](/contexts/credit)
