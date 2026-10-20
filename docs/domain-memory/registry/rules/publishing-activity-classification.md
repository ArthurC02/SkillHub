---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/activity.go#L51-63"
    excerpt_sha256: "sha256:88e1101acbbcb086e639d4936ff4d72b6b40102e0dd902e92dac6f310324c71a"
  - resource: "apps/platform/internal/skill/publishing/activity_test.go#L10-32"
    excerpt_sha256: "sha256:fcd341e8e3d5c525a9ff198c48174168450478bc698f8698554e5b5d2195422e"
---
A publication's activity item asks for attention while it is published without a Release, is recent once published with a Release or delisted, and asks for attention in an unknown status.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A publication with no Release yet shows 尚未建立 Release as needs attention.
