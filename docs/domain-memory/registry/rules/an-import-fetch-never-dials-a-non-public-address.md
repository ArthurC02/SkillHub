---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:08:38.278632Z"
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L50-64"
    excerpt_sha256: "sha256:a401156339274187142e5e156527da96d0cafe0730639e77489815603d7c4640"
  - resource: "apps/platform/internal/skill/admission/fetch_test.go#L286-299"
    excerpt_sha256: "sha256:3829691ef8b0bb1001f332ff646a3d72c8d536b4d2515f96af813e122e0a10c0"
---
An import fetch never connects to a loopback, link-local or metadata address, even through an allowed host name or a redirect.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A redirect into 169.254.169.254 ends with destination address is not allowed.
