---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L103-124"
    excerpt_sha256: "sha256:a9fd0caa2a3f9e3b36f3f43fb71b6789e39a7fc1251707622425c52bbbfbfa0c"
---
A batch answers a trial's questions as the simulated person only when the trial's conversation ended successfully, its test case has facts and the creator still exists; otherwise the questions are left for a real person.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A succeeded run whose test case has no facts gets no simulated reply.
