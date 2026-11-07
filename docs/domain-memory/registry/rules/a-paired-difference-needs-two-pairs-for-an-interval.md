---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T09:48:10.339614Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L110-112"
    excerpt_sha256: "sha256:5357a08d524b6b250c2c6c0b48d8f57c47cc5d16933e98addf275a6b65bd6338"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L134-148"
    excerpt_sha256: "sha256:006809a5dd8211f277ac548a072b08303147719ed40d41e3f78dafff320fd8d3"
---
A paired difference gets a confidence interval only from two or more pairs; one pair gives the difference alone.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- One pair gives a difference with no interval.
