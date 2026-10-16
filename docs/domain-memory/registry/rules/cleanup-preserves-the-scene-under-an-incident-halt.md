---
type: Business Rule
status: stable
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T17:15:48.986336Z"
sources:
  - resource: "apps/platform/internal/trial/execution/cleanup.go#L65-73"
    excerpt_sha256: "sha256:3df07a2ab978833f6417331bc349f4762679b51702218c56b41937d769561b5a"
  - resource: "apps/platform/internal/trial/execution/cleanup.go#L92-97"
    excerpt_sha256: "sha256:34a5845c6fd63f4ba7558553aa8e684d51a8428d6aa6939907109aefc8ff3663"
---
While a P1 incident halt covers the pool or an attempt's provider, cleanup leaves that attempt's sandbox standing and does not mark the Run cleaned, so the scene stays preserved until an operator lifts the halt.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A pool-wide incident halt holds every finished Run's sandbox.
- A halt naming one provider holds only the sandboxes on that provider.
- A capacity pause, which is not an incident, does not hold cleanup.
