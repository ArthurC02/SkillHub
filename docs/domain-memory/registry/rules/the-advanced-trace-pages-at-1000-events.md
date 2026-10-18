---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L424-424"
    excerpt_sha256: "sha256:eef45d0cbbcc4d38c8ef3470c8d5f881a46ca1a704303d85d956b72e7d473838"
  - resource: "apps/platform/internal/trial/evidence/service.go#L441-458"
    excerpt_sha256: "sha256:e37d79c96a2533a5e774c9c53fbdb07f2b5ef679a1bcd3bb3e7d03ed95aba6ca"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_limits_integration_test.go#L12-38"
    excerpt_sha256: "sha256:379aa8de57d909997a3e15c16da88a2fa67b246f5fce2c40031ba9ec69df7811"
---
The advanced trace returns at most 1000 events per page, says more remain, and gives a cursor that continues after the last one.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- Of 1001 events the first page holds 1000 with has_more, and the next page holds the last one.
