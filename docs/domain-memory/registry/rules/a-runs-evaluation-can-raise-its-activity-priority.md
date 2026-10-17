---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:41:43.641716Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L241-249"
    excerpt_sha256: "sha256:bcf9e16a5937ef73ce4265d5eed8cc3c19b269e1152dcb04fe5c3ac2509d1e59"
  - resource: "apps/platform/internal/product/activity/service_test.go#L13-41"
    excerpt_sha256: "sha256:22160df690a73f03f43509e36bd3630fb4668b7eafa730a4bd001ffa6db8c0b1"
---
A Run and its evaluation appear as one item; when the evaluation is more urgent, or equally urgent and newer, its classification and status replace the Run's, and the item's time is the later of the two.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- A succeeded Run at 08:00 whose evaluation is not_met at 09:00 shows as needs attention at 09:00.
