---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L132-147"
    excerpt_sha256: "sha256:1fea6c931f96bc19629c6b4915152a7a472a3b3ae9a235e0a9ba8ced4a9355d1"
  - resource: "apps/platform/internal/trial/batch/batch.go#L153-155"
    excerpt_sha256: "sha256:41da104f7567c86f867dd91a2089fc209ccb3f8af7826ad49d6b71bd9f5860a8"
---
A batch is refused before anything is saved unless it has 1 to 10 trials and 1 to 50 test cases with no test case repeated.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- 11 trials, 51 test cases, or the same test case twice is refused with 400.
