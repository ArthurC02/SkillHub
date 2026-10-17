---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L50-64"
    excerpt_sha256: "sha256:a401156339274187142e5e156527da96d0cafe0730639e77489815603d7c4640"
---
An import fetch never connects to a loopback, link-local or metadata address, even through an allowed host name or a redirect.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A redirect into 169.254.169.254 ends with destination address is not allowed.
