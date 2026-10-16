---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/creation_wiring.go#L110-110"
    excerpt_sha256: "sha256:1af6470cceed3798e1b724bcde33a7d328fcf7203a15022581412dc2aac711b7"
---
# Relations
- Producer context: [testlab](/contexts/testlab)
- Consumer context: [creation](/contexts/creation)
