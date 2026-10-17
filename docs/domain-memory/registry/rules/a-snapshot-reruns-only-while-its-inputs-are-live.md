---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/snapshot.go#L197-211"
    excerpt_sha256: "sha256:dae3569d8fa0614ee5772d9af84acf709d5a70dde6d18e7784cdab10f5dd41b7"
---
A snapshot can be rerun only while its Test Case is not deleted and every dataset it references exists, is not deleted and has not expired.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A dataset expiring at this very moment makes the snapshot not rerunnable.
