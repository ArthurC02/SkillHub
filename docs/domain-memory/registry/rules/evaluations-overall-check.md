---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0004_test_lab_and_runs.sql#L127-127"
    excerpt_sha256: "sha256:891549f23acefef5750d3928dbddaa1d93f5d40af4f36f91c499a358b349ea46"
---
evaluations keeps check evaluations.overall: overall text NOT NULL CHECK (overall IN ('met', 'partially_met', 'not_met', 'undetermined'))

# Relations
- Contexts: [eval](/contexts/eval)
