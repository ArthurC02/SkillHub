---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T09:48:10.339614Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L91-154"
    excerpt_sha256: "sha256:a5b0a5001663cc90d1ef05502b9efae0da906da8d2393986a0e9d78b600e832f"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L150-227"
    excerpt_sha256: "sha256:fd4a67e98e9b4ca1f93116574b5fdea8625ec0cc39bca23eace9fd1e9d2f5f54"
---
A paired difference between two batches, or between a batch and its skill-withheld arm, is presented as better or worse only when its interval excludes zero: improved or worse when the 99.9% interval does, likely improved or likely worse when only the 95% interval does, no clear difference when the 95% interval includes zero, and too few pairs below two pairs; nothing short of likely improved is presented as an improvement.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A mean of +0.19 with a standard error of 0.1 has a 95% interval that includes zero, so the comparison says there is no clear difference rather than better.
- A mean of +0.2 with a standard error of 0.1 is likely improved; +0.33 is improved.
- One paired test case gives a difference but too few pairs for a verdict.
