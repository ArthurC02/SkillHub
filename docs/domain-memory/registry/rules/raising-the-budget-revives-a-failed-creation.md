---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:37:17.368504Z"
sources:
  - resource: "apps/platform/internal/creator/creation/run_fetch_commands.go#L75-77"
    excerpt_sha256: "sha256:9f1c3b470804e0031ab34f87b7800cfa9e8e5b3e37dba9ad0f1ad526fadb7748"
  - resource: "apps/platform/internal/creator/creation/command_test.go#L526-532"
    excerpt_sha256: "sha256:4679675058ef31f6307f1485cc9f1e28e4412f594950f7626c767dda622ada63"
---
Raising the budget of a failed creation returns it to waiting for input; any other state is kept, and no step is queued.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A failed session whose budget is raised waits for input.
