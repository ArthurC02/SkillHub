---
type: Interaction
status: stable
relationship: customer-supplier
consistency: synchronous
delivery: "composition-root fact reader: at dispatch the run context reads, through its registry fact adapter, the unretired mcp_tools rows its Skill Version declares in allowed-tools, and hands their reference, purpose and parameters to the provider as stand-ins"
verified:
  - by: "human:ArthurC"
    at: "2026-11-01T06:02:14.778167Z"
sources:
  - resource: "apps/platform/internal/entrypoint/wiring/trial.go#L30-50"
    excerpt_sha256: "sha256:3a8b7da64bbea465db17d97e785e8e7284019abbef119254221d2f899967088e"
---
# Relations
- Producer context: [ingest](/contexts/ingest)
- Consumer context: [run](/contexts/run)
