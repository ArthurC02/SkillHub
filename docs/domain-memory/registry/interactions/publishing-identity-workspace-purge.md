---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L220-220"
    excerpt_sha256: "sha256:d2f205a22a1246d5467e82a16fd7304a380fed48ca46ba4d2afd1dd2ede232d6"
---
# Relations
- Producer context: [publishing](/contexts/publishing)
- Consumer context: [identity](/contexts/identity)
