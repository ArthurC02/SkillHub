---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L220-220"
    excerpt_sha256: "sha256:d2f205a22a1246d5467e82a16fd7304a380fed48ca46ba4d2afd1dd2ede232d6"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L55-55"
    excerpt_sha256: "sha256:5a4271f7d52e39b4c0b40506defd594d7b6266b1863402aa52f077213e8a3fa4"
---
# Relations
- Producer context: [publishing](/contexts/publishing)
- Consumer context: [identity](/contexts/identity)
