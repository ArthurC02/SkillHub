---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L5-5"
    excerpt_sha256: "sha256:24a3206c8de84f7ca11c4efb10cf0c4c51f5936e56f30cfa886c36af5b956008"
---
run_continuations keeps check run_continuations.questions: questions jsonb NOT NULL CHECK (jsonb_typeof(questions) = 'array' AND jsonb_array_length(questions) > 0)

# Relations
- Contexts: [run](/contexts/run)
