---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/http.go#L97-104"
    excerpt_sha256: "sha256:660d1818ed1354eb83ce0989cc57459a8a1dd887ae60ed4c602a9739e8068d32"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_integration_test.go#L195-218"
    excerpt_sha256: "sha256:765ab21a08d1bd0c33d17038955140154fc195c4c269c5605c7b1586bded7fe4"
---
A trace cursor that is negative or not a number answers 400 in every view.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- after=-1 answers 400.
