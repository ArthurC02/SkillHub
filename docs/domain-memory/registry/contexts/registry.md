---
type: Bounded Context
title: "Skill Registry & Versioning"
status: stable
subdomain: core
implementation_path: skill/library
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/skill/library/doc.go#L1-3"
    excerpt_sha256: "sha256:6065d37fd70c10f03121d0275a55300cba22ec04130f0072338b52ea7e6d0c4f"
---
Owns Skills and their immutable versions: identity, lineage, fork, delete, takedown, and the licensing and redistribution gates on a Skill's materials.

# Requirement prefixes
- SKILL
