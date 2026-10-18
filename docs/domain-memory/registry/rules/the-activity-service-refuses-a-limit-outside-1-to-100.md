---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L162-164"
    excerpt_sha256: "sha256:5839420a6450ef18ca0eeb93d93e43eb2c3feb9cef9bb0d82173d2d097061ea4"
  - resource: "apps/platform/internal/product/activity/limits_test.go#L13-23"
    excerpt_sha256: "sha256:28cbc4d15aa4b8548e38402f2cbd1d324c2292e40d910239a78d8e0b4a48b298"
---
The activity service itself refuses a page limit below 1 or above 100, whatever its caller checked.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- A limit of 101 is refused; 100 is served.
