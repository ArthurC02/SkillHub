---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:08:38.278632Z"
sources:
  - resource: "apps/platform/internal/skill/admission/source.go#L111-136"
    excerpt_sha256: "sha256:efc1523379d5ef2dfa6c218f44e995b78a4af6f5e78cb3203f4f1cf899a72aa6"
  - resource: "apps/platform/internal/skill/admission/source_test.go#L62-98"
    excerpt_sha256: "sha256:878f12779a8feeca1fc38f859cc7ef20321765f8b83fd3f7766bb5ae67a217fd"
---
A Skill that is one directory of a larger package is hashed over its own files only, so the same content hashes the same wherever it sits.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- The same Skill inside two different plugins gets the same content hash, unlike the whole package's digest.
