---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/purge.go#L33-53"
    excerpt_sha256: "sha256:9af317c5e1cd4caa2cf5eb225af44c4de4f6e2a4485052d1d492059a296d2246"
---
Purging a Workspace erases only the Test Cases no snapshot froze, so past runs keep the Test Case they ran.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Of two Test Cases, the snapshotted one survives the purge.
