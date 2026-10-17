---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:49:07.637777Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L187-192"
    excerpt_sha256: "sha256:12701771cebb71792b2f94443da52cf878d92d038ac3c38562bbd05c2e2265c6"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_apply_characterization_test.go#L69-112"
    excerpt_sha256: "sha256:783bbae9a1abdbdfed086e415e58a464701fd669091612052bbd798b3fe0978a"
---
A suggestion applies only while its target file in the newest version is byte-identical to the version it was written against; otherwise it is blocked as target_changed.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Two accepted suggestions on one file: the first applies, the second is refused as target_changed.
