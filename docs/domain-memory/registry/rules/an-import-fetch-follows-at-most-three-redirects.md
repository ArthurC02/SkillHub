---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:08:38.278632Z"
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L106-114"
    excerpt_sha256: "sha256:10556d4399e7ec88b71d2b948ba025a53118e7a2d8568d296e34ef3e7049c97b"
  - resource: "apps/platform/internal/skill/admission/fetch_test.go#L301-318"
    excerpt_sha256: "sha256:406f3dc202c8758bb7273889a031d2afb2e4db82a7b7b718ff8837ba22ea7e30"
---
An import from a URL follows at most three redirects and then stops.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A server that always redirects sees four requests before the import fails.
