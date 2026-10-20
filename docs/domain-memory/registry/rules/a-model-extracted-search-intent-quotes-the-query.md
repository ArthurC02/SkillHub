---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T14:46:47.256731Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/intent.go#L110-122"
    excerpt_sha256: "sha256:fdd85f8191de85094364a7d12c04806f9eb134ff00e317a2849a5e6dcf39c45c"
  - resource: "apps/platform/internal/skill/discovery/intent_test.go#L32-60"
    excerpt_sha256: "sha256:3db3fbfda8811fa7923d1c74325dd60506e8139775f416eb0bd75f9c53e2c481"
---
A search intent the model extracted is accepted only when every filled field appears verbatim in the person's query and none is blank or over 2000 characters; a person's own correction need not quote the original query.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- From the query "CSV", a proposal naming Python as the tool is refused; the person correcting the intent to Python is accepted.
