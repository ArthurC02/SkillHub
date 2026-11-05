---
type: Business Rule
status: stable
owner: skill-packaging
version: "1"
effective_from: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-11-05T16:50:27.141932Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/plugin.go#L108-119"
    excerpt_sha256: "sha256:ca40dea8c42f0d1a16f6dcf607eb0ef6f533c07f47d49e22df7b519cda424dda"
  - resource: "apps/platform/internal/skill/delivery/plugin.go#L167-178"
    excerpt_sha256: "sha256:fd1ed295da7b93e499b54d80618a6c90ba4bfa23c28236ae41ee8b9c1e246b37"
---
A plugin package of several Skill Versions is built only if every member passes the same release checks as a single Skill; the first blocked member stops it and is named in the refusal.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Two members, the second taken down: no plugin is built and the message starts with the second member's name.
- Every member releasable: a zip with plugin.json and skills/<name>/ folders is built and its shape is re-read before it is stored.
