---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/run_fetch_commands.go#L32-60"
    excerpt_sha256: "sha256:360ef5dbd42624609ea908b34b964a46cb3bac78943619903c49874b192ec914"
---
Confirming or declining a fetch is refused unless a fetch permission question is pending.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- confirm_fetch when nobody asked is an invalid command.
