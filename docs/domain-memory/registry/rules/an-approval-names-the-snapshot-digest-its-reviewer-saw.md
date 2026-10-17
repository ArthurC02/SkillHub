---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L505-507"
    excerpt_sha256: "sha256:68596d47432e22a6800c4296c8e56876f5ff2daf3f56204e2e069f8ec4715b1e"
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L522-524"
    excerpt_sha256: "sha256:e345f40f292de8f6a4d16c2b8519d9526293d41d6cc157179ce147f77e0fbfc3"
---
An approval is refused as stale and records nothing unless it names the search snapshot digest the reviewer saw; a revocation needs no such proof.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Text re-enriched while the reviewer reads makes an approval with the old digest fail with 409 and no review row.
