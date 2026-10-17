---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L558-569"
    excerpt_sha256: "sha256:4a2e444d2234a6d7eb1733b0d153d7fbb3c1974a0ba968b5742dca7a3a51e216"
---
A simulated user holds at most MaxSimulatedUserFacts non-blank facts totalling at most MaxSimulatedUserFactsChars characters; anything else is refused.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Eleven facts, a blank fact or one fact over the total is refused; an empty list removes the persona.
