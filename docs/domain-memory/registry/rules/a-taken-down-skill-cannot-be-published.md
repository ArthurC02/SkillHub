---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:06:36.790775Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L238-244"
    excerpt_sha256: "sha256:bdbe4f4dd26d41819129c44f2b7003f3b4dbedfc91561582358b3ec3bbee029f"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_release_guard_integration_test.go#L8-22"
    excerpt_sha256: "sha256:17292075d428fb45abc6a346a7788926e4edc3c8d489ca8f734c75ab8c611ca3"
---
A Skill an operator took down cannot be published; the request is answered not found and no publication is created.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Publishing a taken-down Skill answers 404 and leaves no publication.
