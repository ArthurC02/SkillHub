---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L144-168"
    excerpt_sha256: "sha256:3281488dde4045a795eb11aa1de6abdca7d8db8e283a14adcd5e2e575050c172"
---
A Workspace that may not store objects cannot upload a dataset; the upload is answered not found before anything is stored.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A Workspace being erased uploading rows.csv gets not found and the store receives nothing.
