---
type: Business Rule
status: draft
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
sources:
  - resource: "apps/platform/internal/trial/execution/reassign.go#L16-22"
    excerpt_sha256: "sha256:1be38a39e42fa2076af3d3eabadf3fbc9e0360105df4c9963295ff559cf2f4a2"
  - resource: "apps/platform/internal/trial/execution/reassign.go#L72-90"
    excerpt_sha256: "sha256:bbac22c4ccc604b008785bd7bbc8479943512fb3b660afb8e2216181c299a9dc"
  - resource: "apps/platform/internal/trial/execution/reassign.go#L105-112"
    excerpt_sha256: "sha256:3ca6250a7422e8f2bb66da53b986190203db1024dc84d809ff0b364f3cb872c3"
---
When an attempt's provider is lost the Run is dispatched to another provider once; a second loss fails the Run with provider_error.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A provider that stops answering for longer than the loss window hands the Run to another provider, never back to the one that lost it.
- A second loss ends the Run failed with provider_error and a reason saying it was already reassigned once.
