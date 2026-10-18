---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:31:55.890891Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L98-100"
    excerpt_sha256: "sha256:d8f1c068053ccd8e9733a22ebce9d557d5da5bcbfb94e06f9adcd9e83ced0fb8"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L134-145"
    excerpt_sha256: "sha256:333793f45297c1fac3f3a31a4c3f10dd5adbc015f6fb81408ca3a335da684702"
---
A paired difference gets a confidence interval only from two or more pairs; one pair gives the difference alone.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- One pair gives a difference with no interval.
