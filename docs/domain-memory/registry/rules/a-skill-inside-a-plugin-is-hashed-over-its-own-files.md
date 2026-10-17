---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/admission/source.go#L111-136"
    excerpt_sha256: "sha256:efc1523379d5ef2dfa6c218f44e995b78a4af6f5e78cb3203f4f1cf899a72aa6"
---
A Skill that is one directory of a larger package is hashed over its own files only, so the same content hashes the same wherever it sits.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- The same Skill inside two different plugins gets the same content hash, unlike the whole package's digest.
