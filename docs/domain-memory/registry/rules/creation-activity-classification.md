---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/activity.go#L41-66"
    excerpt_sha256: "sha256:bc9aeb840036ba22202461c72c196b654304b741ee2a51c0b35df2f2fec6febd"
  - resource: "apps/platform/internal/creator/creation/activity_test.go#L5-20"
    excerpt_sha256: "sha256:ec95e4e1f913a2448b16d42b0f712600a564ba04543d8b11b3dc6eb8ddc7d064"
---
A creation session's activity item is in progress while queued or working, asks for attention while it waits for the author (information, confirmation, a draft or candidate to review, a failure, a re-upload), and is recent once saved or cancelled; an unknown state asks for attention.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A session waiting for confirmation shows 等待你的確認 as needs attention; a saved one shows 已儲存 as recent.
