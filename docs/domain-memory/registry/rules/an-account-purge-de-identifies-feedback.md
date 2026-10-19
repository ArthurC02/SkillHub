---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T18:28:57.949881Z"
sources:
  - resource: "apps/platform/internal/product/learning/purge.go#L17-18"
    excerpt_sha256: "sha256:9686aa9ce581038f04d9a0024d00dacf479af0be3a38e66b4999da6d0269a5c5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L587-635"
    excerpt_sha256: "sha256:9ba2ed4f41cdfa8d5b5ceddd880ccb934ef166c8027f305ff4173a2d27211e5e"
---
Purging an account detaches its funnel events from the Workspace and user, and erases its feedback reports' text, page and Run link, keeping only each report's kind and time for counting.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- After the purge a report that said 'I wanted a python profile' on /runs/x keeps its kind need_signal and its time, with no message, page, Run, Workspace or user.
