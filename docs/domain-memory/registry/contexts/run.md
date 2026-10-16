---
type: Bounded Context
title: Run Orchestration
status: stable
subdomain: core
implementation_path: trial/execution
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/trial/execution/doc.go#L1-2"
    excerpt_sha256: "sha256:2258f2bf42ecea1fa2980f67aa7bbfbf667741dc3d73a979e210d503a3397149"
---
Owns a Skill trial Run's lifecycle: its state machine, dispatch to a sandbox provider, following execution to completion, and cleanup.

# Requirement prefixes
- RUN
- SBX
