---
type: Dependency Policy
status: stable
mode: synchronous-query
policy: allowed
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:57:32.208288Z"
sources:
  - resource: "docs/adr/ADR-016-platform-bounded-contexts-and-context-map.md#L42-42"
    excerpt_sha256: "sha256:06f9cce7957b49b6423ae078092651cbf450d73418736b008dd5cabb50f939b4"
---
Every user-data query resolves its Workspace scope here; this is the single entry point that makes workspace scoping enforceable.

# Relations
- From context: [testlab](/contexts/testlab)
- To context: [identity](/contexts/identity)
