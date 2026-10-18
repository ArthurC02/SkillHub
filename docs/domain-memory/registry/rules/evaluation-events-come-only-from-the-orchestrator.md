---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/event.go#L138-139"
    excerpt_sha256: "sha256:3942e9f42ca84b1395ba6c09f526f6eb7640743a2e855f01550771612ccdb90f"
  - resource: "apps/platform/internal/trial/evidence/evaluation_event_test.go#L40-48"
    excerpt_sha256: "sha256:e7ae8465c931788850f0c23f8d54e31aa1a5e1c6098712a162031c9016e67de2"
---
evaluation_started and evaluation_completed events are refused unless the orchestrator emitted them, even from an otherwise known producer.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An evaluation_completed event emitted by the sandbox is refused as invalid.
