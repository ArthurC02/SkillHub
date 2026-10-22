---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0101_run_failure_class_resource_limit.sql#L3-13"
    excerpt_sha256: "sha256:93580f49ff2229ca25168cb507b7da28ef17b5391b332dec7a6e4198c2fbca5f"
---
runs keeps check runs_failure_class_check: CONSTRAINT runs_failure_class_check CHECK (failure_class IS NULL OR failure_class IN ( 'provider_error', 'workload_error', 'timeout', 'cancelled', 'capability_mismatch', 'policy_refused', 'platform_error', 'resource_limit' ))

# Relations
- Contexts: [run](/contexts/run)
