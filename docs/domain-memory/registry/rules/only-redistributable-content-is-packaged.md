---
type: Business Rule
status: stable
owner: skill-packaging
version: "1"
effective_from: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/packaging.go#L317-335"
    excerpt_sha256: "sha256:9952ba0b34a11eb51db88d64f7a13399d531b68c14a488ca1331848d6622cab2"
  - resource: "docs/adr/ADR-007-packaging-license-provenance-and-redistribution.md#L54-60"
    excerpt_sha256: "sha256:1b0006fa5933a886673e33699c6644e8c808c4b511baf7344a7c9e10c1d678e5"
---
Only a Skill whose redistribution verdict is allowed, self_supplied or generated may be packaged; blocked gives not_redistributable and every other value, including an empty one, is treated as license_unknown.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- redistribution = allowed, self_supplied or generated: the build proceeds.
- redistribution = blocked: refused as not_redistributable.
- redistribution = '' or an unrecognised string: refused as license_unknown, unconfirmed license counts as not allowed.
