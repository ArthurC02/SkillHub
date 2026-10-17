---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/entitlements/quota.go#L101-101"
    excerpt_sha256: "sha256:1797f3509bbc09245a741eb54d357af291c9c7282d56820b78049db85fe4efb6"
  - resource: "apps/platform/internal/product/entitlements/quota.go#L182-186"
    excerpt_sha256: "sha256:fe837607a4ccd37e7b2399326cc71dea687a3995d3f525b67ef993c3b9964d32"
---
The daily allowance counts the runs of the 24 hours before now, not of the calendar day.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A run at 23:00 yesterday still counts at 10:00 today.
