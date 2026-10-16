---
type: Bounded Context
title: Publishing
status: stable
subdomain: core
implementation_path: skill/publishing
requirement_prefixes: []
verified:
  - by: "human:ArthurC"
    at: "2026-09-26T14:16:50.511970Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/doc.go#L1-3"
    excerpt_sha256: "sha256:9b3298692a9a6e75f33b854c2bdd5116312aff5379e9ac778626f8167e5dceb4"
---
Owns who discloses what under which name: a creator's publisher namespace, the publications and Bundles in it, their immutable releases, and the operator reviews that expose a release to search.
