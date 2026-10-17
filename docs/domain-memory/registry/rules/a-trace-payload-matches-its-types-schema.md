---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:45:46.007817Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/payload.go#L189-200"
    excerpt_sha256: "sha256:db9ab627e5864940a9756f1373cd3248304a7502178de444dfa04bd182cecb27"
  - resource: "apps/platform/internal/trial/evidence/payload_contract_test.go#L223-254"
    excerpt_sha256: "sha256:8064080b3e97895756f108dc8b9aa0becab303bfafae2d476d185331d46c9fb9"
---
Each event type declares its payload schema; missing required fields, wrong kinds, values outside an enum and undeclared fields on a closed type are refused.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An agent_output payload with an extra field is refused.
