---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T09:48:10.339614Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L42-77"
    excerpt_sha256: "sha256:7c8767e568418548ba47be6eaeb6df116d7d1a19af372e49e365f0fab72c83d3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L584-625"
    excerpt_sha256: "sha256:be958287f9a866c0b2b004bc4e8a7c40acdd29f230c6d25f0ec218b98f22387c"
---
Two batches are compared only on test cases judged in both; a test case judged in only one counts as unpaired, not as better, worse or tied.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case judged in the candidate but not the reference adds to unpaired.
