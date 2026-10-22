---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0089_exposure_reviews.sql#L21-23"
    excerpt_sha256: "sha256:0a21496c7bdb085d8cd0ed7f7801421b9ff407b565df2c60e53d492c78c0cc1f"
---
exposure_reviews keeps trigger exposure_reviews_immutable: CREATE TRIGGER exposure_reviews_immutable BEFORE UPDATE OR DELETE ON exposure_reviews FOR EACH ROW EXECUTE FUNCTION enforce_immutable()

# Relations
- Contexts: [publishing](/contexts/publishing)
