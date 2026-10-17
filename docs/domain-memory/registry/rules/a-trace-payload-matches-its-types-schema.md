---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/payload.go#L189-200"
    excerpt_sha256: "sha256:db9ab627e5864940a9756f1373cd3248304a7502178de444dfa04bd182cecb27"
---
Each event type declares its payload schema; missing required fields, wrong kinds, values outside an enum and undeclared fields on a closed type are refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An agent_output payload with an extra field is refused.
