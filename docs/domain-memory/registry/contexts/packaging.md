---
type: Bounded Context
title: "Packaging & Distribution"
status: stable
subdomain: core
implementation_path: skill/delivery
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/doc.go#L1-3"
    excerpt_sha256: "sha256:1711ee5492b86b4bcbb56112334fe0fa0ea64b6bc04fdb8d134df091643d6b96"
---
Turns an immutable Skill Version into a downloadable package: it builds the archive, writes the manifest, and owns the download artifacts and records produced from it.

# Requirement prefixes
- PACK
