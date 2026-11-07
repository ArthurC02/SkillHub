---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T19:36:24.856168Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L241-246"
    excerpt_sha256: "sha256:49b9adc2e6092ce09d12d265adae9b22deac7a8987c982fa985dab0d7e79c25a"
  - resource: "apps/platform/internal/trial/improvement/apply_limits_characterization_test.go#L11-26"
    excerpt_sha256: "sha256:f6543c327f6a0e2382e9b4190b13b59de84fb524a176273fb020a6cc9e2232b5"
---
A target file with a NUL byte in its first 8000 bytes is not diffable, so its suggestion is blocked as diff_unavailable rather than applied unseen.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A target with a NUL at offset 7999 is refused as not text.
