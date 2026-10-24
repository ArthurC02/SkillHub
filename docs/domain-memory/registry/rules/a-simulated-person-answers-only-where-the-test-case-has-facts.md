---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_simulated_user.go#L109-130"
    excerpt_sha256: "sha256:a9fd0caa2a3f9e3b36f3f43fb71b6789e39a7fc1251707622425c52bbbfbfa0c"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_simulated_user_integration_test.go#L79-113"
    excerpt_sha256: "sha256:6c5df44dca15ccf4b7f9877ffc4b88b50436d234cf06b4828dda18a6ebc23a5f"
---
A batch answers a trial's questions as the simulated person only when the trial's conversation ended successfully, its test case has facts and the creator still exists; otherwise the questions are left for a real person.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A succeeded run whose test case has no facts gets no simulated reply.
