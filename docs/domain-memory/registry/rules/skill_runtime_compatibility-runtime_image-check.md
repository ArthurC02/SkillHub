---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0022_agent_compatibility.sql#L4-4"
    excerpt_sha256: "sha256:fde5c18efa1c665af01af2149aa3158d07289b9661883d28c17c19e1225bc023"
---
skill_runtime_compatibility keeps check skill_runtime_compatibility.runtime_image: runtime_image text NOT NULL CHECK (runtime_image <> '')

# Relations
- Contexts: [registry](/contexts/registry)
