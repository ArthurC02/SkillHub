---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:08:38.278632Z"
sources:
  - resource: "apps/platform/internal/skill/admission/fetch.go#L106-114"
    excerpt_sha256: "sha256:10556d4399e7ec88b71d2b948ba025a53118e7a2d8568d296e34ef3e7049c97b"
  - resource: "apps/platform/internal/skill/admission/source_url.go#L24-35"
    excerpt_sha256: "sha256:accc9465f4a37eed465279710dfbc6c2ba66acdce0d0aadc4739a8475f6288d6"
  - resource: "apps/platform/internal/skill/admission/fetch_test.go#L63-78"
    excerpt_sha256: "sha256:25cab0958feaac3154cc0182902010af65a6c1c2b3c9cf314579b1b1c9152211"
---
Each redirect of an import fetch is checked against the allowed hosts again, so a redirect off the list is never followed.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- An allowed host that redirects to another host is refused, and the other host receives no request.
