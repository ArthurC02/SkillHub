---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/diff.go#L140-143"
    excerpt_sha256: "sha256:194475fe21d195276d75825d26aebd2393980e2d5d254c6d50a7210d68269019"
  - resource: "apps/platform/internal/skill/library/content_guards_test.go#L76-84"
    excerpt_sha256: "sha256:90713c817e3b4f2fd0f259d5dd869b9a475c79f89435f87435525fa8018a6fef"
---
A file larger than 1 MiB on either side gets no text diff; exactly 1 MiB is diffed.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- Two 1 MiB files are diffed; a file one byte over is not.
