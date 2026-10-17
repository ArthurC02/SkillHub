---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L187-192"
    excerpt_sha256: "sha256:12701771cebb71792b2f94443da52cf878d92d038ac3c38562bbd05c2e2265c6"
---
A suggestion applies only while its target file in the newest version is byte-identical to the version it was written against; otherwise it is blocked as target_changed.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Two accepted suggestions on one file: the first applies, the second is refused as target_changed.
