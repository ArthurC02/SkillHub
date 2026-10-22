---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0118_evaluation_batches.sql#L6-6"
    excerpt_sha256: "sha256:8fa9fb81709de10e5b78caf7f58c68e9d73ef2be01671c86c32dd4e11bb192e9"
---
evaluation_batches keeps check evaluation_batches.trials: trials integer NOT NULL CHECK (trials BETWEEN 1 AND 10)

# Relations
- Contexts: [batch](/contexts/batch)
