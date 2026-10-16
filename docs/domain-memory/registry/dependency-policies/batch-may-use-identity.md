---
type: Dependency Policy
status: stable
mode: synchronous-query
policy: allowed
verified:
  - by: "human:ArthurC"
    at: "2026-10-11T18:23:06.041403Z"
sources:
  - resource: "docs/adr/ADR-016-platform-bounded-contexts-and-context-map.md#L42-42"
    excerpt_sha256: "sha256:06f9cce7957b49b6423ae078092651cbf450d73418736b008dd5cabb50f939b4"
---
Every batch request resolves its Workspace scope here; this is the single entry point that makes workspace scoping enforceable.

# Relations
- From context: [batch](/contexts/batch)
- To context: [identity](/contexts/identity)
