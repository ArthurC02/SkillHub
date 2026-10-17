---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L640-642"
    excerpt_sha256: "sha256:f5065104f232d1879c455e454199732e37886328586f6043ec3eb55a1c6a2b53"
---
A Test Case holds at most MaxCriteria acceptance criteria; one more is refused as over the limit.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- The 50th criterion is accepted and the 51st refused.
