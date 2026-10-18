---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:31:55.890891Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L84-86"
    excerpt_sha256: "sha256:cced6a85752bee1d61e7e9e7f7c24d6d2285d27306c3585337c0689f625129ed"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_scope_integration_test.go#L8-33"
    excerpt_sha256: "sha256:31e162ab9981a55bb6fcd43e06b766753242ecac7f10481eba52fe718dfb40e3"
---
A batch report asked for under another Skill of the same Workspace answers not found.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Reading a batch under a second Skill id gives 404.
