---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/money.go#L30-38"
    excerpt_sha256: "sha256:162827f2762093738e339d20f7e465adaffa055963a7f61a2e32ac9c760b6e78"
  - resource: "apps/platform/internal/creator/credit/money_test.go#L53-68"
    excerpt_sha256: "sha256:f40d29758968210065f48ce455a8fa7049ff271fbf129dd5bc8a016b3f580f7a"
---
A cost or markup outside its billable range is refused with ErrAmountOutOfRange instead of overflowing, and billing rounds up, never down.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Pricing an amount above MaxBillableMicros is an error, not a bill of 0.
