---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T07:03:29.672830Z"
sources:
  - resource: "docs/adr/ADR-023-account-purge.md#L48-48"
    excerpt_sha256: "sha256:1a2ba40a85527c67fa6e66244ee305cdb4f8880568e86bcae7b64714982bbbe4"
  - resource: "apps/platform/internal/skill/library/aggregate_test.go#L269-312"
    excerpt_sha256: "sha256:812cfd1760d4c8999649c26baba6a5b56e8f5cdf4babf8dd45bef22a8ec876d4"
---
A skill version row never changes except once, to erase its content: that one update sets content_erased_at and must leave the manifest empty and the import source cleared, may change nothing else, and freezes the row again.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Erasing while rewriting the content hash, or while keeping the manifest, is refused; after the erasure, restoring a manifest, un-erasing or erasing again is refused.
