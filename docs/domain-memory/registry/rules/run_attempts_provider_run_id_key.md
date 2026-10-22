---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0016_run_orchestration.sql#L18-19"
    excerpt_sha256: "sha256:fabbecec34d0f7a04e40bffbcba4f3166abea5284f01fc2dee53d280200a5f45"
---
run_attempts keeps unique-index run_attempts_provider_run_id_key: CREATE UNIQUE INDEX run_attempts_provider_run_id_key ON run_attempts (provider, provider_run_id) WHERE provider_run_id IS NOT NULL

# Relations
- Contexts: [run](/contexts/run)
