---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L219-219"
    excerpt_sha256: "sha256:875e512562f8552c6e2fe2e0c217a9d4a27edbebffd97420dd9e08bb452b1473"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L54-54"
    excerpt_sha256: "sha256:9262f7ba28b120c8a9d9b0201190a2e30e5d1b7c3931cba06acf3c836ec4cb12"
---
# Relations
- Producer context: [eval](/contexts/eval)
- Consumer context: [identity](/contexts/identity)
