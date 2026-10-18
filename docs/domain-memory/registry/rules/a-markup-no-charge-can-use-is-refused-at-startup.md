---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T08:44:17.756466Z"
sources:
  - resource: "apps/platform/internal/creator/credit/config.go#L36-38"
    excerpt_sha256: "sha256:1c9caa6ab661e4a6896755f1c9e187e90055e57a31f9674b9fddf3595ac10f41"
  - resource: "apps/platform/internal/creator/credit/config_test.go#L67-71"
    excerpt_sha256: "sha256:9e4236288350d16db066209e538719a146f3b1d873b6b451509172d7494ce4fe"
---
Configuration refuses a markup above the largest one a charge can bill, so the service never starts with every call unbillable.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- A markup of 1,000,001 bps is refused; 1,000,000 is accepted.
