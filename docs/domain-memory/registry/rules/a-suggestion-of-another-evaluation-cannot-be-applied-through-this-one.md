---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_apply_characterization_test.go#L44-67"
    excerpt_sha256: "sha256:6147934408299054a3ba62ce6962f2500ec7e8ab2019aa32c3cbd13df2fd53a2"
  - resource: "apps/platform/internal/trial/improvement/apply.go#L501-506"
    excerpt_sha256: "sha256:9aa8b79f20a0fe4fc838a0c5c6b14335c0033ba7e73bccca9d18350da04398ea"
---
A suggestion that belongs to another evaluation is not found when applied through this one.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Suggestion of evaluation E1 posted with E2 answers 404.
