---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L218-233"
    excerpt_sha256: "sha256:3483bedc0ce415a180da9f790563e11b5726d1c32b1bfc3e78049de39fa13965"
  - resource: "apps/platform/internal/skill/publishing/exposure_test.go#L160-194"
    excerpt_sha256: "sha256:f698ef064c02717f552f2f724f72d4e3e3198368b49223d53f011d4d30672e62"
---
An exposure approval is refused unless the publication is published, its Skill is available and cleared for the Catalog, and a fully enriched search snapshot of the released version exists.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Approving while the snapshot is still enriching is refused as snapshot_pending; approving a generated Skill whose latest release declares no licence is refused as redistribution_not_allowed.
