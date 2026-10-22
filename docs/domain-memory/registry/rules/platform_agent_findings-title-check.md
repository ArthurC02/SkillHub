---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0112_platform_agent_findings.sql#L6-6"
    excerpt_sha256: "sha256:9c137b16a3992705f8d3e7aeaa3fb95a77f9b72025b4124b42ccb232a4c44e41"
---
platform_agent_findings keeps check platform_agent_findings.title: title text NOT NULL CHECK (btrim(title) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
