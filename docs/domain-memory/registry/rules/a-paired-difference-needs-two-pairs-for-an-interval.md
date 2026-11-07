---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T09:48:10.339614Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L129-131"
    excerpt_sha256: "sha256:5357a08d524b6b250c2c6c0b48d8f57c47cc5d16933e98addf275a6b65bd6338"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L134-148"
    excerpt_sha256: "sha256:d389308c82fc9f2a8cdad17992a31dd4b9e1cee86464f7cd37c9dda41abb7e3a"
---
A paired difference gets a confidence interval only from two or more pairs; one pair gives the difference alone.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- One pair gives a difference with no interval.
