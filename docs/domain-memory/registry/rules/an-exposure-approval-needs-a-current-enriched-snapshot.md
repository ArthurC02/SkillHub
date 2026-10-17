---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:50:56.766681Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/exposure.go#L210-225"
    excerpt_sha256: "sha256:d90e79154ab3ae7bf683f1b82983f3b40105749dd7661835c6851271c517d7f0"
  - resource: "apps/platform/internal/skill/publishing/exposure_test.go#L154-184"
    excerpt_sha256: "sha256:81ed59ce43ecdd925ff09a91718f7fad8dab1d7c5a78c8ca28b75ce9860c056a"
---
An exposure approval is refused unless the publication is published, its Skill is available and cleared for redistribution, and a fully enriched search snapshot of the released version exists.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Approving while the snapshot is still enriching is refused as snapshot_pending.
