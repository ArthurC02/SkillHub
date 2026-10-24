---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L666-671"
    excerpt_sha256: "sha256:ad04bbcbe3c53aec2b1dbd5a2a209b61e0090301a8317daced9b5751c2b5d7e7"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/testlab_integration_test.go#L288-336"
    excerpt_sha256: "sha256:bae90962e97deaa23ea147e30d898dc4753b459d3d3442ee034faa585c13ba48"
---
Changing the text of an acceptance criterion clears its confirmation, so a person must confirm the new wording.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- A confirmed criterion edited to add a unit comes back unconfirmed.
