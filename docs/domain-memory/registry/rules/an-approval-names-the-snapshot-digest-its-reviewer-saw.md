---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L512-514"
    excerpt_sha256: "sha256:68596d47432e22a6800c4296c8e56876f5ff2daf3f56204e2e069f8ec4715b1e"
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L529-531"
    excerpt_sha256: "sha256:e345f40f292de8f6a4d16c2b8519d9526293d41d6cc157179ce147f77e0fbfc3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/exposure_integration_test.go#L347-394"
    excerpt_sha256: "sha256:8ac74fd716f64426d304597ba245515b9142ebdf7bafbcdce309af3f92778ecd"
---
An approval is refused as stale and records nothing unless it names the search snapshot digest the reviewer saw; a revocation needs no such proof.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Text re-enriched while the reviewer reads makes an approval with the old digest fail with 409 and no review row.
