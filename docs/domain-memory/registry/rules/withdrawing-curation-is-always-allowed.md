---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L309-334"
    excerpt_sha256: "sha256:0be422fbe37411f8965e9fff5d979978ec4697662ae03ac344c9f498db31a4fc"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L201-246"
    excerpt_sha256: "sha256:17c908727240d50a32ce3ff9bf0a434ad68d64f882c9c26e650dfeaec0d021aa"
---
Withdrawing curation is allowed for any skill, inside a catalogue or not, and clears the curated version.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A curated skill that left the catalogue is withdrawn and its curated version cleared.
