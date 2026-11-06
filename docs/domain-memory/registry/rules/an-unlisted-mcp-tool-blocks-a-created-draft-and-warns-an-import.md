---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/skill/admission/mcp_tools.go#L13-43"
    excerpt_sha256: "sha256:0dff15123f2cc103abfe2bd0a5a7540b2416c27b6570b8d23e26da3978f12a0c"
  - resource: "apps/platform/internal/skill/admission/mcp_tools.go#L54-65"
    excerpt_sha256: "sha256:df93ec0b6bdce85c9b44c6c99dc719a3f139cf39d9f3285af182dfa00ff04277"
  - resource: "apps/platform/internal/skill/admission/creation.go#L52-52"
    excerpt_sha256: "sha256:5248e297b8dda66dcf2785374f181b041475027ace06330c3bb3db3fbf4f716c"
  - resource: "apps/platform/internal/skill/admission/creation.go#L208-208"
    excerpt_sha256: "sha256:5248e297b8dda66dcf2785374f181b041475027ace06330c3bb3db3fbf4f716c"
  - resource: "apps/platform/internal/skill/admission/service.go#L341-341"
    excerpt_sha256: "sha256:06b9214ed080db7e866e502be192d11fe6152c484835061148a4e929f0848258"
  - resource: "apps/platform/internal/skill/admission/mcp_tools_test.go#L35-57"
    excerpt_sha256: "sha256:025365e193db70c470ad49a71c9016770caea2709cbab2e3ac8d07d8a8714085"
  - resource: "apps/platform/internal/skill/admission/service.go#L415-415"
    excerpt_sha256: "sha256:06b9214ed080db7e866e502be192d11fe6152c484835061148a4e929f0848258"
---
A created draft or created version of a Skill that declares in allowed-tools an mcp__ name no mcp_tools row offers is blocked with the finding mcp-tool-not-offered naming it and listing the offered references; an imported or uploaded package gets the same finding as a warning and is not blocked.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A created draft declaring mcp__mail__send while mcp_tools is empty is blocked and told the list is empty.
- An imported zip declaring mcp__mail__send is admitted with a warning.
