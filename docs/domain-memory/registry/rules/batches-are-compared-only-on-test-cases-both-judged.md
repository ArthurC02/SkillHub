---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:13:02.301694Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L41-76"
    excerpt_sha256: "sha256:289ea79783d392a422187574d2ecaae36382cfd20626f8283a53d5220aa9edb6"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L493-533"
    excerpt_sha256: "sha256:250e1b3f1c63f2f11b7e19e47ad5d8af3de9494a054b646dcc39a83f35724e2c"
---
Two batches are compared only on test cases judged in both; a test case judged in only one counts as unpaired, not as better, worse or tied.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A test case judged in the candidate but not the reference adds to unpaired.
