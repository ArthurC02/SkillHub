---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:31:55.890891Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L236-237"
    excerpt_sha256: "sha256:ac8b39d16d7b9da4d6810cba23e252c41be1f8cd6cbaf3de6652331c53f2a7f0"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L69-89"
    excerpt_sha256: "sha256:3934f014a914057bb1330226d9fc2734d2990e0ba38c84b4f72bf317d68bdf06"
---
pass@k is true once any trial passes, false only when all k are judged and none passed, and unknown otherwise.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- One passed trial with the rest running gives pass@k true.
