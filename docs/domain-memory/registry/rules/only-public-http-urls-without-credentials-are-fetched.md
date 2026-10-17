---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/webfetch/fetch.go#L38-54"
    excerpt_sha256: "sha256:bb0bdd2751e1170f36a5886c187087eaea7c52b190601ec03dd18ee4149aa65f"
---
Only http or https URLs with a public host and no credentials can be fetched; a literal private address is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- http://10.0.0.1/ and http://user:pw@host/ are refused.
