---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:16:09.817174Z"
sources:
  - resource: "apps/platform/internal/creator/creation/webfetch/fetch.go#L38-54"
    excerpt_sha256: "sha256:bb0bdd2751e1170f36a5886c187087eaea7c52b190601ec03dd18ee4149aa65f"
  - resource: "apps/platform/internal/creator/creation/webfetch/fetch_test.go#L15-25"
    excerpt_sha256: "sha256:f1c60c5534694653cf1dd7b42c7c8c5476dd19a5205de389078658291d03120c"
---
Only http or https URLs with a public host and no credentials can be fetched; a literal private address is refused.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- http://10.0.0.1/ and http://user:pw@host/ are refused.
