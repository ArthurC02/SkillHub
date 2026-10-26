---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L185-204"
    excerpt_sha256: "sha256:944af754f6d1e8ff987861ed0d04a7cfad3805bd4d7693325e5173637e28bbee"
  - resource: "apps/platform/internal/skill/admission/service_test.go#L69-108"
    excerpt_sha256: "sha256:89439b462bb8a8de4b5dc1c96b788f3dc43fc5758ad8b7eac23bdb78af753759"
---
A version is written only when its source carries the provenance its kind requires: git needs a URL and no generator fields, an upload has neither, a generated source needs a model and prompt version and no URL; anything else is refused with ErrIncompleteProvenance.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- An upload claiming a URL is refused with ErrIncompleteProvenance.
