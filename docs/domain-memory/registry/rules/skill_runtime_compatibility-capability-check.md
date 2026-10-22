---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0022_agent_compatibility.sql#L6-7"
    excerpt_sha256: "sha256:9adf43ae2568a7559899c17fb8d46e0aec743708ffb8eb50a0f38b5213ce6a63"
---
skill_runtime_compatibility keeps check skill_runtime_compatibility.capability: capability text NOT NULL CHECK (capability IN ('activated', 'not_activated', 'unverified'))

# Relations
- Contexts: [registry](/contexts/registry)
