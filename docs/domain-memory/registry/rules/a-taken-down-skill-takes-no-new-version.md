---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L598-605"
    excerpt_sha256: "sha256:53f5f9b2099c41148ef9a142ef94d92174b27dac9b06b600288e071e6d590501"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/version_refusal_integration_test.go#L48-77"
    excerpt_sha256: "sha256:fa35029bb6012585eab2761d785cdea396c5435601f97c2865a997ac98c872b9"
---
A taken-down Skill refuses every new version as skill_taken_down and gains no version row; in a re-import of several Skills the others still import.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- After a takedown, uploading tidy-notes again is blocked and it keeps one version.
