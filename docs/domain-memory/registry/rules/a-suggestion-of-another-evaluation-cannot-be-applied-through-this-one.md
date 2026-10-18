---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L477-482"
    excerpt_sha256: "sha256:60af0f4ad1dc8b9d63b511db3c49b512d853327b31db7bc1d30647b890cb0dd1"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_apply_characterization_test.go#L44-67"
    excerpt_sha256: "sha256:6147934408299054a3ba62ce6962f2500ec7e8ab2019aa32c3cbd13df2fd53a2"
---
A suggestion that belongs to another evaluation is not found when applied through this one.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Suggestion of evaluation E1 posted with E2 answers 404.
