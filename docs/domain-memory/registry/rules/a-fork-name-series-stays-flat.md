---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/registry.go#L266-297"
    excerpt_sha256: "sha256:370b4cccfe01d3f9eaa253c64b7e4bdf6c58f019fe3d42658e5b5111e9812517"
  - resource: "apps/platform/internal/skill/library/write_test.go#L49-69"
    excerpt_sha256: "sha256:6cfde76915fc31804b1ed9849100a56eb036fedd970dcf1b35d56e8f90aca1e5"
---
A fork of a fork reuses the base name instead of growing it, treating only an ordinal of two or more as a series number.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Forking tidy-csv-fork-2 proposes tidy-csv-fork, while tidy-csv-fork-1 is a plain name.
