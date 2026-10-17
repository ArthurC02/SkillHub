---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L219-224"
    excerpt_sha256: "sha256:bd538d6f219337858e3f5f9d60751d6a0c5ea9dc0f933e547f6cc24760ca02e7"
---
A target file with a NUL byte in its first 8000 bytes is not diffable, so its suggestion is blocked as diff_unavailable rather than applied unseen.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A target with a NUL at offset 7999 is refused as not text.
