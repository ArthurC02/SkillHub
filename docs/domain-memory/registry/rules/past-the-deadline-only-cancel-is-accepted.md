---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L300-300"
    excerpt_sha256: "sha256:f18748987f2dc8901f183287cca2004725112a4579485583626d70c758cccad5"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L89-101"
    excerpt_sha256: "sha256:39620236efec7f89bb8f5c3c05f3db7cdda7f5c37ed70450849a9068be7aabb3"
---
After a creation session's deadline only cancel and the three save commands (materialize, finalize, confirm duplicate), none of which calls the model, are accepted; every other command is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A draft-ready session past its deadline saves; a message or a budget raise is refused.
