---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:06:36.790775Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L413-415"
    excerpt_sha256: "sha256:4c82a61280b22d24f9eb043161b1cf71c44bfbdaae39c767d9be8779b05c343c"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/exposure_integration_test.go#L305-339"
    excerpt_sha256: "sha256:7b742849aada15e4ee3cdfb0bd32ac1679e59301ee34662faeaa5674f6077a7e"
---
An exposure review is refused as stale unless it names the current release and the review sequence its reviewer saw.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Two reviewers open sequence 0; after one approves, the other's revocation at sequence 0 is refused as stale.
