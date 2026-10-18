---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/general.go#L12-14"
    excerpt_sha256: "sha256:ef2bf7ad590106d2e2ac53418d4bfa64571b3225702bfd36a636dce2190e0869"
  - resource: "apps/platform/internal/trial/evidence/general.go#L44-48"
    excerpt_sha256: "sha256:54ef7061c57b49ac0e8bab330f470d10687befdbe377f0f439c6294dc01568eb"
  - resource: "apps/platform/internal/trial/evidence/general_test.go#L21-40"
    excerpt_sha256: "sha256:f41a8f5f999f41ca044563d09fdf7fb9445054b742ea0962c5fa172424208ef9"
---
The general trace lists at most the first 100 skills used while still counting all of them.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A run using 101 skills lists 100 and counts 101.
