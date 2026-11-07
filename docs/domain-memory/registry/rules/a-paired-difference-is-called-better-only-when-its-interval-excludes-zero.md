---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T09:48:10.339614Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L100-134"
    excerpt_sha256: "sha256:a32f124e038f39b6528efe8e188f2f4923c4d7061ff9fa60b51600770d57cffb"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L150-183"
    excerpt_sha256: "sha256:0f5a0ffc9ceebca6b6b380b5b54b7b5e0825a271a4392056265e4a16d474d6e9"
---
A paired difference between two batches, or between a batch and its skill-withheld arm, is presented as better or worse only when its interval excludes zero: improved or worse when the 99.9% interval does, likely improved or likely worse when only the 95% interval does, no clear difference when the 95% interval includes zero, and too few pairs below two pairs; nothing short of likely improved is presented as an improvement.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A mean of +0.19 with a standard error of 0.1 has a 95% interval that includes zero, so the comparison says there is no clear difference rather than better.
- A mean of +0.2 with a standard error of 0.1 is likely improved; +0.33 is improved.
- One paired test case gives a difference but too few pairs for a verdict.
