---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L223-225"
    excerpt_sha256: "sha256:525759144f2f7793822c7fd283cc4e2ffc92209360ccf0494b12bae5a1898428"
  - resource: "apps/platform/internal/trial/batch/outcome_counting_test.go#L11-26"
    excerpt_sha256: "sha256:d818be8c9f35feaca48ef9e3b5ec92d3b94e52a973c2fd36884d3ad8f340327c"
---
A trial counts as passed only when its outcome is met; a partially met trial is judged but not passed.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Met, partially met and not met give 1 passed of 3 judged.
