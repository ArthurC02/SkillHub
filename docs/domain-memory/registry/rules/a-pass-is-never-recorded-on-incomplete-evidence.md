---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/judging/judge.go#L306-310"
    excerpt_sha256: "sha256:545cd336e59db78beb013d83b95a0ca4c680d69eb6537dfbc97c1460313bc81b"
  - resource: "apps/platform/internal/trial/improvement/judging/judge_test.go#L450-479"
    excerpt_sha256: "sha256:3aec672df7d96812cf79d496b6062eda7ffb4b42779064c768349c7517953678"
---
A passed criterion is recorded as undetermined when the trace is incomplete, the input was cut, or the evidence rests on a trimmed source.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A judge pass on a run with trace gaps is stored undetermined.
