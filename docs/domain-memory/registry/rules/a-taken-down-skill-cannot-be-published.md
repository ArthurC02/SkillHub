---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L238-244"
    excerpt_sha256: "sha256:bdbe4f4dd26d41819129c44f2b7003f3b4dbedfc91561582358b3ec3bbee029f"
---
A Skill an operator took down cannot be published; the request is answered not found and no publication is created.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Publishing a taken-down Skill answers 404 and leaves no publication.
