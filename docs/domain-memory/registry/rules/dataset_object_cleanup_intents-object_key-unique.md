---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0047_dataset_object_cleanup_intents.sql#L4-4"
    excerpt_sha256: "sha256:3f2b5cbf7df23c6c1aa04b4dd4f9a116756ea4f0f12cf282125c38cf612145a2"
---
dataset_object_cleanup_intents keeps unique dataset_object_cleanup_intents.object_key: object_key text NOT NULL UNIQUE

# Relations
- Contexts: [testlab](/contexts/testlab)
