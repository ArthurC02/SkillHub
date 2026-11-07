---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_comparison.go#L56-60"
    excerpt_sha256: "sha256:74862fafdb4566856e4276d335bd59d6ba79c0b54bbec79fe609d8af017489c0"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L217-237"
    excerpt_sha256: "sha256:55c1fc85fff16548ca3046c087f45671cc142dba44d70a31fdf4ff06b719622f"
---
A paired test case counts as better only when its rate is strictly higher, worse when strictly lower, and tied when equal.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Rates 1 and 0.5 count as better; equal rates count as tied.
