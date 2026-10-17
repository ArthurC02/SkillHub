---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:46:14.276516Z"
sources:
  - resource: "apps/platform/internal/trial/design/purge.go#L33-53"
    excerpt_sha256: "sha256:9af317c5e1cd4caa2cf5eb225af44c4de4f6e2a4485052d1d492059a296d2246"
  - resource: "apps/platform/internal/trial/design/purge_test.go#L10-28"
    excerpt_sha256: "sha256:21927454a7740370d3acbb5edc616fda5f26f5254e3a2488a1b683ecf8347133"
---
Purging a Workspace erases only the Test Cases no snapshot froze, so past runs keep the Test Case they ran.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Of two Test Cases, the snapshotted one survives the purge.
