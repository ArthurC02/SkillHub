---
type: Bounded Context
title: "Evaluation & Improvement"
status: stable
subdomain: core
implementation_path: trial/improvement
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/doc.go#L1-3"
    excerpt_sha256: "sha256:34298e555d583899df7b5373146b88ec0f8ffb9d49e55b8efe3e214ad72b6ce7"
---
Judges a completed trial Run against its acceptance criteria, lets a creator compare Runs and review improvement suggestions, and applies accepted suggestions as a new Skill Version.

# Requirement prefixes
- EVAL
