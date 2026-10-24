---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L416-423"
    excerpt_sha256: "sha256:ce1ed5eb3f399d531ca86580f30822b1cc6e5b82892bd2a5f49606c128e2022e"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L147-168"
    excerpt_sha256: "sha256:5dd8e61af210a147411209b8d1b40e8fa5a1f6188d13f45d46b7dfa2205cc0e7"
---
A person's message is refused when blank or longer than 4000 characters.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- 4000 characters are accepted; 4001 are refused.
