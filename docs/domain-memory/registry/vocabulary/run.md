---
type: Term
title: Run
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T09:19:06.131820Z"
sources:
  - resource: "docs/adr/ADR-003-run-orchestration-and-async-workflows.md#L8-8"
    excerpt_sha256: "sha256:dc42b71eceff8e533c671d0f7cbce4203a3221566d25c99508d90727b124f67b"
---
One requested trial of a single Skill Version against a single frozen Test Case snapshot, within a Workspace, carried out through one or more attempts on a sandbox Provider, and ending in a single terminal status. An attempt is the execution; the Run is the retryable unit whose identity a reassignment does not change.

# Relations
- Contexts: [run](/contexts/run)
