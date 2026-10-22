---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L19-19"
    excerpt_sha256: "sha256:9ad04392dcbc9412a26cf4948590ba94bb8266cc803769255be9cf84caabfbb8"
---
evaluation_grader_versions keeps check evaluation_grader_versions.instructions: instructions text NOT NULL CHECK (btrim(instructions) <> '' AND length(instructions) <= 4000)

# Relations
- Contexts: [eval](/contexts/eval)
