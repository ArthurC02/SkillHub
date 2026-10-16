---
type: Bounded Context
title: "Trust & Supply Chain"
status: stable
subdomain: core
implementation_path: skill/admission
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/skill/admission/doc.go#L1-3"
    excerpt_sha256: "sha256:50cd5e0780f26f20d4c9fa996aeaf6196507abaa623568e4f8f74aa03c0a427a"
---
Owns admitting an outside Skill package into the platform: validating its structure statically, storing the accepted archive, and recording where it came from.

# Requirement prefixes
- SKILL
- SEC
