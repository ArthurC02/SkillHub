---
name: domain-memory-read
description: Read a reviewed file-backed Domain Registry to guide implementation with DDD terms, ownership, invariants, and boundaries.
---

# Read Domain Memory

Use this before coding when a request uses a domain term or may cross a Context.

1. In an unfamiliar repository, run `readiness --repo-root <repo>` first. It is read-only and routes only: `empty` and `greenfield` need source discovery or Design before any Registry read, and `dead` needs recovery. Do not call `probe` as the first command.
2. Run `probe --repo-root <repo> --registry-root <root>`.
3. Run `validate --registry-root <root> --repo-root <repo>` and validate the policy.
4. Resolve terms with `resolve-terms`, then inspect the owner with `get-context` and `get-record`.
5. Inspect existing collaboration with `analyze-boundary` before proposing a new dependency.
6. Before coding, answer the tactical design questions in [tactical design reasoning](../../references/tactical-reasoning.md). Decide whether a tactical pattern is needed, then choose an idiomatic implementation only after identifying the owner, invariant, consistency boundary, and failure behavior.
7. For a material change, emit the [implementation handoff](../../references/implementation-handoff.md) with reviewed facts, forces, unknowns, and proof obligations. Do not invent the decision in Read mode when the change needs Design.

Use `reviewed` records as constraints. Candidate records are Working Memory: use them to find terms, owners, evidence, and questions to verify, but never present them as settled policy or use them to authorize a material change. Treat stale sources, unverified maps, and missing records as gaps to report. Do not write files in this mode. See [the file-backed API](../../references/script-api.md).
