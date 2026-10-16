---
type: Dependency Policy
status: stable
mode: synchronous-write
policy: allowed
verified:
  - by: "human:ArthurC"
    at: "2026-10-13T07:59:14.973768Z"
sources:
  - resource: "docs/adr/ADR-016-platform-bounded-contexts-and-context-map.md#L42-42"
    excerpt_sha256: "sha256:06f9cce7957b49b6423ae078092651cbf450d73418736b008dd5cabb50f939b4"
  - resource: "apps/platform/internal/trial/improvement/eval.go#L429-434"
    excerpt_sha256: "sha256:8e4497ec3cdbf2478f1c7c57bb68071cc444ccf76a09177495a2d9c9426e96c5"
---
Judging and comparing Runs reads their recorded evidence, and an evaluation records its own start and end as orchestrator events in its transaction.

# Relations
- From context: [eval](/contexts/eval)
- To context: [trace](/contexts/trace)
