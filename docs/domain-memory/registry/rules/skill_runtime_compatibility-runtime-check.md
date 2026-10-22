---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0022_agent_compatibility.sql#L9-10"
    excerpt_sha256: "sha256:a63b6405d9d744601ca777e55c49f839918b19572b583b9646adb01ea9232e0f"
---
skill_runtime_compatibility keeps check skill_runtime_compatibility.runtime: runtime text NOT NULL CHECK (runtime IN ('native', 'transpiled', 'failed', 'unverified'))

# Relations
- Contexts: [registry](/contexts/registry)
