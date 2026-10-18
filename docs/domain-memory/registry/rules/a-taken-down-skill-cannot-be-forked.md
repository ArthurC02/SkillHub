---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/registry.go#L95-97"
    excerpt_sha256: "sha256:dc731f5fb105b3dbe00230e749236725a65ea88871386489208a38bfbfd7886b"
  - resource: "apps/platform/internal/skill/library/fork_source_characterization_test.go#L62-114"
    excerpt_sha256: "sha256:8cadd6e93638b685c9abf4ae4b6661edc008c4f7e525c6d7bb9ae9f0f983e18a"
---
Forking a taken-down skill finds nothing.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Forking a catalogue skill an operator took down answers not found.
