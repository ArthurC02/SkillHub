---
name: domain-memory-read
description: Read a reviewed file-backed Domain Registry to guide implementation with DDD terms, ownership, invariants, and boundaries.
---

# Read Domain Memory

Use this before coding when a request uses a domain term or may cross a Context.

1. Run `probe --repo-root <repo> --registry-root <root>`.
2. Run `validate --registry-root <root> --repo-root <repo>` and validate the policy.
3. Resolve terms with `resolve-terms`, then inspect the owner with `get-context` and `get-record`.
4. Inspect existing collaboration with `analyze-boundary` before proposing a new dependency.

Use only `reviewed` records as constraints. Treat candidates, stale sources, unverified maps, and missing records as gaps to report. Do not write files in this mode. See [the file-backed API](../../references/script-api.md).
