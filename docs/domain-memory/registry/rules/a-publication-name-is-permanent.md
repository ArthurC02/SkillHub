---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:50:56.766681Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L363-366"
    excerpt_sha256: "sha256:e197ee981b6023dafa0149a50c26fa20ab36699df6979099986488e75f20db72"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_integration_test.go#L345-372"
    excerpt_sha256: "sha256:20d078f0a78ffdc40021e043e094514e80b33b410c63fac6d159093dd05b12b7"
---
A Skill that already has a publication cannot be republished under another name; the request is refused as name_is_permanent and the publication keeps its name.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A Skill published as foo and republished as another-name answers 409 name_is_permanent.
