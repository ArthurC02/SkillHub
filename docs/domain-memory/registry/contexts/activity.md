---
type: Bounded Context
title: Workspace Activity
status: stable
subdomain: supporting
implementation_path: product/activity
requirement_prefixes: []
verified:
  - by: "human:ArthurC"
    at: "2026-09-30T09:52:50.115174Z"
sources:
  - resource: "contracts/openapi/public.yaml#L381-425"
    excerpt_sha256: "sha256:6fbd05ffa8416562abd6f9fb395642a16cdf25feb6dc1babf9c69a68bff4c3df"
---
Owns the Workspace-scoped cross-source activity query: contracted source coverage, completeness, global ordering, cursor semantics and typed continuation composition, but no source lifecycle state.
