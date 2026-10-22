---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0078_run_activity_read_from_finished_at.sql#L11-12"
    excerpt_sha256: "sha256:8a1f1ee051a50c3f3f8b9e0f0a32ff040ef409dc3525de64bf124ffaa38ab8e7"
---
runs keeps check runs_finished_exactly_when_terminal: CONSTRAINT runs_finished_exactly_when_terminal CHECK ((finished_at IS NOT NULL) = (status IN ('succeeded', 'failed', 'cancelled', 'timed_out')))

# Relations
- Contexts: [run](/contexts/run)
