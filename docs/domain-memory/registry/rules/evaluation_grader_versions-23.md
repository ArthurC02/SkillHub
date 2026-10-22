---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0119_evaluation_graders.sql#L23-23"
    excerpt_sha256: "sha256:3a9177ea8dc742ad6302bc992def8d0d2af73a27ae8df74175d8611bf33f11c3"
---
evaluation_grader_versions keeps unique evaluation_grader_versions:23: UNIQUE (grader_id, version)

# Relations
- Contexts: [eval](/contexts/eval)
