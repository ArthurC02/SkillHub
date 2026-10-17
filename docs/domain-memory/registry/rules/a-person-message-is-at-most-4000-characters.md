---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L413-420"
    excerpt_sha256: "sha256:ce1ed5eb3f399d531ca86580f30822b1cc6e5b82892bd2a5f49606c128e2022e"
---
A person's message is refused when blank or longer than 4000 characters.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- 4000 characters are accepted; 4001 are refused.
