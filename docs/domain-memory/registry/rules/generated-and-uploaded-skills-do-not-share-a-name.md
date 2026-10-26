---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T01:48:28.064585Z"
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L459-462"
    excerpt_sha256: "sha256:931f0de54d52ccff3324c35a11df0139dd7eccbf6cd9fb0decd044f1d1818009"
  - resource: "apps/platform/internal/skill/admission/source.go#L318-320"
    excerpt_sha256: "sha256:6ee66eeb8798f18a22d1de950c509ccfaea129be8dee28cfb00e525d0c48aa5d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/generate_integration_test.go#L488-495"
    excerpt_sha256: "sha256:5ce31c71c263ee1ef54534269a8a334f268caa2920d31bfe374a76d27861c4db"
---
A name held by a generated Skill takes no uploaded version, and a generated candidate takes no name held by an uploaded Skill; either way the save is refused with ErrGeneratedNameCollision.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Generate pdf-extract, then upload a package named pdf-extract: refused.
