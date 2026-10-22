---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0018_run_scheduling.sql#L1-9"
    excerpt_sha256: "sha256:6980ecd90e21e47d20b7e06de785c45fb7585fca1d2d202dae6fde73d85f5b3e"
---
runs keeps check runs.failure_class: failure_class text CHECK (failure_class IS NULL OR failure_class IN ( 'provider_error', 'workload_error', 'timeout', 'cancelled', 'capability_mismatch', 'platform_error' ))

# Relations
- Contexts: [run](/contexts/run)
