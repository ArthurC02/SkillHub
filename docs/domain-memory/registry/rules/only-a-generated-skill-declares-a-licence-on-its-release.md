---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T00:09:09.697460Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/publication.go#L97-108"
    excerpt_sha256: "sha256:de84963b0d6c4a4c506ec8ef2564f57de511f7e14ca91527596a810b73b3719a"
  - resource: "apps/platform/internal/skill/publishing/publication_test.go#L86-113"
    excerpt_sha256: "sha256:7669391636791d0546726dccc47c0629f2cd76edebdc4af9268afb24b6f4b950"
---
An author may declare a licence when releasing a Skill only if the Skill was generated for them, and only one of the known SPDX identifiers, spelled exactly; the declaration is kept on that release.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Releasing a generated Skill with MIT stores MIT on the release; WTFPL, mit, or MIT on a self-supplied Skill is refused as license_not_declarable.
