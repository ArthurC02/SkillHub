---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:31:55.890891Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L56-60"
    excerpt_sha256: "sha256:74862fafdb4566856e4276d335bd59d6ba79c0b54bbec79fe609d8af017489c0"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L216-236"
    excerpt_sha256: "sha256:75f0cd960cb1b9c1e9adcc19f4f351000e07487d423f3d18b8e8ce6039740baa"
---
A paired test case counts as better only when its rate is strictly higher, worse when strictly lower, and tied when equal.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Rates 1 and 0.5 count as better; equal rates count as tied.
