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
  - resource: "apps/platform/internal/skill/delivery/plugin.go#L92-101"
    excerpt_sha256: "sha256:d2183a3f4e3b72d329a357d34901e649bd6d33f468589f38e9b4e9228007540b"
  - resource: "apps/platform/internal/skill/delivery/plugin.go#L144-154"
    excerpt_sha256: "sha256:e3b8ff470f3a70d10ec51b54844764941dc2a8cf11953c786d8a24e8de279b63"
---
A plugin package of several Skill Versions is built only if every member passes the same release checks as a single Skill; the first blocked member stops it and is named in the refusal.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Two members, the second taken down: no plugin is built and the message starts with the second member's name.
- Every member releasable: a zip with plugin.json and skills/<name>/ folders is built and its shape is re-read before it is stored.
