---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0113_platform_agent_proposals.sql#L10-11"
    excerpt_sha256: "sha256:7228a9974a02c6a9e19d04d49b307326d8919826e64ff39f16808809319928e7"
---
platform_agent_proposals keeps check platform_agent_proposals.status: status text NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'approved', 'rejected', 'expired', 'running', 'succeeded', 'failed'))

# Relations
- Contexts: [operations](/contexts/operations)
