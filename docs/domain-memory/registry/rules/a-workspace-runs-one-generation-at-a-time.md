---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/generate.go#L589-597"
    excerpt_sha256: "sha256:d431c298671dcdefb3d22f0c8dc9903348fb46e6106d187ff9f1729c36a40a77"
  - resource: "apps/platform/internal/skill/admission/generate_test.go#L225-254"
    excerpt_sha256: "sha256:fcbbb89bfa343d25d1dbbfe6dc212a687de11b8f139cab321d0a0dba9b77214d"
---
A workspace holds at most one generation at a time across every API instance: a second request while a lease is live is refused as in flight, another workspace is unaffected, and an expired lease can be taken over.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Two API instances asking for the same workspace: the first holds the slot and the second is refused; a different workspace still generates.
