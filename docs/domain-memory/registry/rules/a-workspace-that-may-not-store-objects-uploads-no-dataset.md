---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:46:14.276516Z"
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L144-168"
    excerpt_sha256: "sha256:3281488dde4045a795eb11aa1de6abdca7d8db8e283a14adcd5e2e575050c172"
  - resource: "apps/platform/internal/trial/design/dataset_characterization_test.go#L149-165"
    excerpt_sha256: "sha256:3aa5882e18df2e392d68420b3eb6eed5c61cc4fa816297e7919f30762ddf9e4a"
---
A Workspace that may not store objects cannot upload a dataset; the upload is answered not found before anything is stored.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A Workspace being erased uploading rows.csv gets not found and the store receives nothing.
