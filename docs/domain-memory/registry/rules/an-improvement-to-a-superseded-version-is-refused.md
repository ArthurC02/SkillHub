---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L473-475"
    excerpt_sha256: "sha256:877e1de1f9a1fa1816ad63f5889d876c48d6072e838c76eb54d421f1593edb1b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L788-840"
    excerpt_sha256: "sha256:5303accac73762162a167fd6feb02560d8d71f28cb1f3e8b86c7e1794032e280"
---
Saving an improved version is refused with ErrBaseVersionMoved when the Skill's newest version is no longer the version the improvement was made to; content identical to an existing version is deduplicated first.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Suggestions applied to v1 after v2 was saved are refused as target_changed.
