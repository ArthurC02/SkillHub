---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L9-10"
    excerpt_sha256: "sha256:553d37b3edadd50e985532c2ff08e08c5a5cceb269a13a77c219f53ae23a59a1"
---
run_continuations keeps check run_continuations_answer_every_question: CONSTRAINT run_continuations_answer_every_question CHECK (jsonb_array_length(answers) = jsonb_array_length(questions))

# Relations
- Contexts: [run](/contexts/run)
