---
type: Business Rule
status: stable
owner: skill-packaging
version: "1"
effective_from: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T17:57:56.233858Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/download.go#L208-213"
    excerpt_sha256: "sha256:3921f2feaca8a3c98c9f662b9a56005332ae665be7e00034e1192e795c52ab6f"
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L997-1010"
    excerpt_sha256: "sha256:f62289177ebc15e1b203887af0068e6be773c63b43cae072b7630b31c67a3831"
---
A stored package is served only while the Skill is still releasable: a taken-down, held or non-redistributable Skill makes its already built packages unavailable for download and shows them as withheld in the list.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A package built yesterday, then the Skill is taken down: the content route answers not found and the list shows serve_state withheld.
- A plugin package is withheld if any of its member Skill Versions is held, taken down or not redistributable.
