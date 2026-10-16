---
type: Dependency Policy
status: stable
mode: none
policy: forbidden
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T11:40:29.217542Z"
sources:
  - resource: "docs/adr/ADR-016-platform-bounded-contexts-and-context-map.md#L42-42"
    excerpt_sha256: "sha256:06f9cce7957b49b6423ae078092651cbf450d73418736b008dd5cabb50f939b4"
---
Entitlements are decided from facts the asking Context supplies, so the rule holder never resolves scope for itself.

# Relations
- From context: [policy](/contexts/policy)
- To context: [identity](/contexts/identity)
