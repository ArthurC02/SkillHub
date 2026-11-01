---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-01T04:29:21.537405Z"
sources:
  - resource: "apps/platform/internal/skill/admission/mcp_tools.go#L21-31"
    excerpt_sha256: "sha256:c27a56a58650c2a60d81826e07c17618b1129aeff89bb6429ee81a367a8ddfec"
  - resource: "apps/platform/internal/skill/admission/mcp_tools_test.go#L59-70"
    excerpt_sha256: "sha256:9ebff8f4db7234811f8fccbbdd98132a993ac2a3a3e4831bf6b3c8c64ebfd858"
---
When the mcp_tools list cannot be read, the allowed-tools check of a package that declares an mcp__ name fails with an error and gives no verdict on the name, instead of passing the package.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A package declaring mcp__calendar__create_event, checked while the database is unreachable, returns an error and no findings.
