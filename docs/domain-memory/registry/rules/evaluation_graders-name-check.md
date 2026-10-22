---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L4-4"
    excerpt_sha256: "sha256:e1198cd057fbac215c56ef412c392fed76cb20042b5743718b80a49d2eacc2fb"
---
evaluation_graders keeps check evaluation_graders.name: name text NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 100)

# Relations
- Contexts: [eval](/contexts/eval)
