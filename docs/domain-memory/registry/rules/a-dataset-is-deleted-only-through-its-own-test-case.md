---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:46:14.276516Z"
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L334-349"
    excerpt_sha256: "sha256:a7ea17c49ccdd871b48eb4c57b678a7f69a5b9c431b18987134bc9530de172ff"
  - resource: "apps/platform/internal/trial/design/dataset_ownership_integration_test.go#L262-287"
    excerpt_sha256: "sha256:9a237874c5f117457a0d4e857b7918f337f54f0a115c32fc5e3a475e3f4d83b7"
---
A dataset can be deleted only through the Test Case that owns it; naming another Test Case finds nothing and removes nothing.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Deleting Test Case B's dataset under Test Case A answers not found and B's file stays.
