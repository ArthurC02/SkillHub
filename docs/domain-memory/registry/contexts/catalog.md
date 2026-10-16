---
type: Bounded Context
title: "Catalog & Discovery"
status: stable
subdomain: core
implementation_path: skill/discovery
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/doc.go#L1-3"
    excerpt_sha256: "sha256:ce32d825feff2cf05012f0a904be7fa8b1da74a7eb3ee3ecb197e132846e7dd2"
---
Owns Skill discovery: the search-facing projection of a Skill, hybrid lexical and vector ranking, and the tier, trust, licence and redistribution facets shown alongside a result.

# Requirement prefixes
- DISC
