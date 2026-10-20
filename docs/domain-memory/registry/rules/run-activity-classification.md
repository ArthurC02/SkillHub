---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/activity.go#L67-90"
    excerpt_sha256: "sha256:1c93f2f43103fc2e815462ad6d34419fd1f33e23fd98841f60f4643c354e2e29"
  - resource: "apps/platform/internal/trial/execution/activity_test.go#L9-33"
    excerpt_sha256: "sha256:9b52d79005254edfed5ce14ff2766b17662728fd0d534d4362dafa55a4da0ca4"
---
A Run's activity item asks for attention when it failed or timed out, is in progress from queued through evaluating, and is recent once it succeeded or was cancelled; a status the Run context does not know asks for attention.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A timed-out Run shows 試跑逾時 as needs attention; a cancelled one shows 已取消 as recent.
