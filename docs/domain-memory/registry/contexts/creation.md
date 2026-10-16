---
type: Bounded Context
title: Skill Creation
status: stable
subdomain: core
implementation_path: creator/creation
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/creator/creation/doc.go#L1-2"
    excerpt_sha256: "sha256:5946916feb42af8748934caf0f39eeecde3cd335a9cd64d46bc91a3603151354"
---
Owns interactive Skill-creation sessions: their steps, checkpoints, and the proposals produced along the way.

# Requirement prefixes
- GEN
