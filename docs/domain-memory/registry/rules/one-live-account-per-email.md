---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L71-73"
    excerpt_sha256: "sha256:a3bc73b95a85ac3220605a631f5a6a58741106dab883c4539b3ed0424c045d84"
  - resource: "apps/platform/internal/creator/workspace/service.go#L200-209"
    excerpt_sha256: "sha256:f0faa6d0e7ace9070133c48e727de6d0ed2e179adc560f0f95d0d5db7e91f646"
  - resource: "apps/platform/internal/creator/workspace/signup_integration_test.go#L73-96"
    excerpt_sha256: "sha256:e7a14bc510df45d790bb87a03107f79a00d271e91c4413000cceb3e691d6edcc"
  - resource: "apps/platform/internal/creator/workspace/http.go#L651-661"
    excerpt_sha256: "sha256:4bec91ea7625b1777382abf7b06318eb74ea928f17d40d962c29391ce039eb25"
---
Two live accounts never share an email: emails are compared after trimming and lower-casing, a sign-up whose email belongs to a live account is refused, and an email held only by a deleted account can be used again.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Signing up as ' Alice@Example.COM ' while alice@example.com belongs to a live account is refused as email-taken (HTTP 409).
- Signing up with the email of an account that is already deleted succeeds and yields a session.
