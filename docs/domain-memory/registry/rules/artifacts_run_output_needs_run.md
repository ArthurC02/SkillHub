---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L155-155"
    excerpt_sha256: "sha256:1ffe9908bcbc6030939d61f6ce923d29d36dff01db57487e5b5817dac46fbb0b"
---
artifacts keeps check artifacts_run_output_needs_run: CONSTRAINT artifacts_run_output_needs_run CHECK (kind <> 'run_output' OR run_id IS NOT NULL)

# Relations
- Contexts: [run](/contexts/run)
