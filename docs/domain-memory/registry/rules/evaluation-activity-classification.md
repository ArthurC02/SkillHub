---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T12:36:17.110562Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/activity.go#L51-74"
    excerpt_sha256: "sha256:92ef9d4677e2cf698050f79a81cc178b4d783e9f7d07a84503cc346940dbf659"
  - resource: "apps/platform/internal/trial/improvement/activity_test.go#L13-38"
    excerpt_sha256: "sha256:ce5e3931b66f793638f2faacee13b18c3fe0825b50a334d845e33f8e2a43202c"
---
An evaluation's activity item is in progress while pending, recent only when completed as met, and asks for attention when it failed or completed as partially met, not met or undetermined; any other status and verdict pair is unsupported and asks for attention.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A completed evaluation that partially met its criteria shows 部分符合驗收標準 as needs attention.
