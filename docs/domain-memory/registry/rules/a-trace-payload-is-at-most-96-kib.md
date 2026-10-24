---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L145-146"
    excerpt_sha256: "sha256:4752673520f87440b59698125fd5e529da315fc4aca58ca12c6e71e82f80f398"
  - resource: "apps/platform/internal/trial/evidence/limits_characterization_test.go#L33-39"
    excerpt_sha256: "sha256:06875357a15c572d7e463ace8a087e6b7b64fa2736aa0993a2bc8ac95791b9b4"
---
A trace event whose payload exceeds 96 KiB is refused as invalid; exactly 96 KiB is accepted.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A payload of 98305 bytes is refused.
