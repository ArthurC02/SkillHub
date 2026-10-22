---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L87-88"
    excerpt_sha256: "sha256:1cffea28825745eed865232aebe9707e718a149fe637d4e8455f9993f3921ec2"
---
runs keeps unique-index runs_provider_run_id_key: CREATE UNIQUE INDEX runs_provider_run_id_key ON runs (provider, provider_run_id) WHERE provider_run_id IS NOT NULL

# Relations
- Contexts: [run](/contexts/run)
