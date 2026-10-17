---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L773-793"
    excerpt_sha256: "sha256:1632c80882599b638ca6df20af86d766fd20903472010e09299d63a1c44c57e0"
---
Before any package object is stored, a Workspace that may not store objects is answered not found and nothing is written.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A Workspace whose lifecycle forbids storage gets 404 and no object.
