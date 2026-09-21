---
name: domain-memory-maintain
description: Keep file-backed Domain Memory current after implementation or source changes by preserving evidence and creating governed candidate updates.
---

# Maintain Domain Memory

Use this after implementation and whenever a selected source moves.

For a brownfield refactoring that preserves the reviewed model, follow the
[brownfield refactoring fast path](../../references/brownfield-refactoring.md)
to decide whether a candidate is needed before creating one.

1. Run `verify-sources`, `verify-evidence`, and `verify-audit`.
2. Compare the implemented behavior with the reviewed terms, owner, invariants, boundaries, events, contracts, and capabilities.
3. Compare the implementation's tactical choice, including an explicit choice of no pattern, with the invariant, ownership, consistency, retry, and failure behavior it claims to preserve. Review behavior and dependencies rather than a fixed language pattern. Use [tactical design reasoning](../../references/tactical-reasoning.md).
4. If the model changed, update the smallest affected asset with `upsert-candidate`, or prepare a Change Package for a material change.
5. Validate the complete Registry and preserve the audit trail.

For a material change, compare the implementation with its implementation handoff and run the counterfactual check described in [pattern verification](../../references/pattern-verification.md). Missing proof, a changed assumption, or a different consistency decision is a model discrepancy even when the unit tests are green.

Never hide a domain change in code-only edits. Candidates prepare the next review and do not authorize implementation. Use only the controlled file-backed commands; do not hand-edit reviewed records or policy. Read [registry maintenance](../../references/registry-maintenance.md).
