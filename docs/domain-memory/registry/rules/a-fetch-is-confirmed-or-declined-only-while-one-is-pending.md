---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/creation/run_fetch_commands.go#L32-60"
    excerpt_sha256: "sha256:360ef5dbd42624609ea908b34b964a46cb3bac78943619903c49874b192ec914"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L483-492"
    excerpt_sha256: "sha256:a5b266aa0fdc0520d59634cfe0aefde3930e8beafddb83473ed5e8dc9852a6b5"
---
Confirming or declining a fetch is refused unless a fetch permission question is pending.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- confirm_fetch when nobody asked is an invalid command.
