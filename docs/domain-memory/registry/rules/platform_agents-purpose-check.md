---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0109_platform_agents.sql#L4-4"
    excerpt_sha256: "sha256:41386cb59b5f088e157ea40da84cf6c8fc0b775065a3cc27979e80de0e434af9"
---
platform_agents keeps check platform_agents.purpose: purpose text NOT NULL CHECK (btrim(purpose) <> '')

# Relations
- Contexts: [operations](/contexts/operations)
