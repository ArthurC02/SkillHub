---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/admission/generate.go#L74-85"
    excerpt_sha256: "sha256:e858cbcf5fd4385f333895d17cc356675966f12b0e265bdb73e4b631f001a2e1"
  - resource: "apps/platform/internal/skill/admission/generate_test.go#L208-215"
    excerpt_sha256: "sha256:84f8a180da9f01b5196376ae8a40319289a44c24d5896a64829d5a83addb3cc0"
---
A generation's task description counts characters, not bytes: one shorter than 8 characters without a diagram is refused as blank, one longer than 4000 is refused as too long, and both bounds themselves are accepted; with no description, a diagram is required.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Eight letters are accepted, seven are refused as blank; 4000 CJK characters are accepted, 4001 are refused.
