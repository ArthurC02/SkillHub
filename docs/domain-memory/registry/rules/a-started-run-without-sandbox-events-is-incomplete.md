---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L57-62"
    excerpt_sha256: "sha256:949a54b286775eed380efd3862ce150eae433600f695d9b6562f5a123458efa2"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L311-333"
    excerpt_sha256: "sha256:b0b6f7d8ae87c4c9a452df1a5e17d0f1171c1c81449fee5203a61d9a3e208389"
---
A trace is not complete when the run started but no sandbox stream exists; a run that never started is not flagged.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A succeeded run with only orchestrator events reads as incomplete.
