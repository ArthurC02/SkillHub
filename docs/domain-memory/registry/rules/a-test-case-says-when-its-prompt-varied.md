---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L228-228"
    excerpt_sha256: "sha256:fe231245072d9916891c87e20577cb9eeaf9226fd25498b4a0792aa09f76a2b7"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L377-440"
    excerpt_sha256: "sha256:3f237b1777a5de90cab5c471d129c6993bbdd565ea58c2219063162e9ecd78ba"
---
A test case is marked prompt-varied only when its trials started from more than one distinct prompt.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Trials started from two different prompt hashes are marked varied.
