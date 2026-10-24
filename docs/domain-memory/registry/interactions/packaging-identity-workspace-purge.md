---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L221-221"
    excerpt_sha256: "sha256:70a173d82b2084fc3ef5a8c12d92b0e906fb2377aee147656437a51492fd3066"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L64-64"
    excerpt_sha256: "sha256:647cec0721b44499df4242fb0b21ac2dd2a9e7d5e3f7f0d0dfcb1c4bf9a06769"
---
# Relations
- Producer context: [packaging](/contexts/packaging)
- Consumer context: [identity](/contexts/identity)
