---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L69-71"
    excerpt_sha256: "sha256:5147efde9535d9d029557db52bc53d164b3b6d42c0fcfafb4329f546d4029e34"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L694-711"
    excerpt_sha256: "sha256:fed2319cb9d340487d77f75e1bd5423d87ae5e169f8ff944d5019a141be03b53"
---
Recording a cost event ignores the caller's cancellation and runs under its own deadline, so a client disconnect cannot lose a cost event.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Recording with an already-cancelled request still stores the event.
