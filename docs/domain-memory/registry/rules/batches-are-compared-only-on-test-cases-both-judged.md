---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L41-76"
    excerpt_sha256: "sha256:289ea79783d392a422187574d2ecaae36382cfd20626f8283a53d5220aa9edb6"
---
Two batches are compared only on test cases judged in both; a test case judged in only one counts as unpaired, not as better, worse or tied.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case judged in the candidate but not the reference adds to unpaired.
