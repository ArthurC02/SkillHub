---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T03:31:47.051282Z"
sources:
  - resource: "apps/platform/internal/creator/creation/creation.go#L310-312"
    excerpt_sha256: "sha256:99edc738823808d985ab780dacd6b00dd9c746d18c73e021d80bd260dcfb3f6a"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L63-70"
    excerpt_sha256: "sha256:5c71942845a7cfc3d5623519a6ac04a54f89488d74553cfb17de9688f9a4cdec"
---
A creation command is refused as a conflict unless its expected revision equals the stored one.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A command for revision 2 against revision 3 conflicts.
