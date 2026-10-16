---
type: Bounded Context
title: Test Lab
status: stable
subdomain: core
implementation_path: trial/design
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/trial/design/doc.go#L1-3"
    excerpt_sha256: "sha256:59f3d24b0abfb539df1f38e697bffd2d84ea729a253a893dcf3af4f0706a23bd"
---
Owns Test Case drafts — their prompts, acceptance criteria, rubrics and uploaded datasets — and freezes a draft into the immutable snapshot a Run executes against.

# Requirement prefixes
- TEST
