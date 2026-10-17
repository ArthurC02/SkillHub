---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:45:46.007817Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/mask.go#L195-201"
    excerpt_sha256: "sha256:36de434ffc195b0aa5f8628624eba770148f07ebbcf1010bfc765a68b2038fe5"
  - resource: "apps/platform/internal/trial/evidence/limits_characterization_test.go#L41-51"
    excerpt_sha256: "sha256:a44ff74a053c36fb8c356eb2a37c2d1ee25b7010a3a05c02ee8784754eabf120"
---
A known secret such as the Run's own ingestion token is scrubbed from every payload when it is at least 16 bytes long; shorter known values are left alone so ordinary text is not redacted.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A 16-byte token inside a log message becomes [REDACTED]; a 15-byte one stays.
