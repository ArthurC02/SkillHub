---
type: Business Rule
status: stable
owner: skill-packaging
version: "1"
effective_from: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T17:57:56.233858Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L306-316"
    excerpt_sha256: "sha256:c4d142e9ef53b8137e2256042130849453bfa1ca7a771920b2b0dd33e0c5b45d"
---
A Skill that the platform has taken down, or whose content is held for an unresolved license question, cannot be built into a package; the refusal names taken_down or license_hold.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A taken-down Skill: building any version answers blocked_reason taken_down.
- A Skill with access_restricted set and not taken down: blocked_reason license_hold.
- A Skill that is both taken down and restricted reports taken_down, because the taken-down check runs first.
