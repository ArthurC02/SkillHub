---
name: domain-memory-maintain
description: Keep file-backed Domain Memory current after implementation or source changes by preserving evidence and creating governed candidate updates.
---

# Maintain Domain Memory

Use this after implementation and whenever a selected source moves.

For a brownfield refactoring that preserves the reviewed model, follow the
[brownfield refactoring fast path](../../references/brownfield-refactoring.md)
to decide whether a candidate is needed before creating one.

1. Run `verify-sources --repo-root <repo> --source-map <registry>/source-map.json --policy <registry>/domain-memory-policy.json`, `verify-evidence --registry-root <registry> --repo-root <repo>`, and `verify-audit --registry-root <registry>`.
2. Compare the implemented behavior with the reviewed terms, owner, invariants, boundaries, events, contracts, and capabilities.
3. Compare the implementation's tactical choice, including an explicit choice of no pattern, with the invariant, ownership, consistency, retry, and failure behavior it claims to preserve. Review behavior and dependencies rather than a fixed language pattern. Use [tactical design reasoning](../../references/tactical-reasoning.md).
   When deployment configuration affects an immutable record's behavior, verify that the chosen value is captured in that record's snapshot rather than reread during retries.
   Integration tests must construct adapters through the same composition wiring; do not retain Context-owned environment factories only for tests.
   Treat HTTP clients, transports, and timeouts as adapter composition concerns: inject them at the entrypoint and retain a focused seam test that proves the selected client reaches the adapter.
   A test-friendly adapter fallback does not replace production composition; verify the executable entrypoint supplies the client explicitly.
   When an adapter depends on deployment posture, parse posture once at the entrypoint and pass the typed value to wiring instead of letting the adapter reread environment variables.
4. If the model changed, update the smallest affected asset with `upsert-candidate`, or prepare a Change Package for a material change. For a boundary-preserving refactor, retain the focused architecture check when one exists; otherwise retain affected tests and lint as preservation evidence.
5. From the repository root, run `validate --registry-root <registry> --repo-root <repo>`, then preserve the audit trail.

For a material change, compare the implementation with its implementation handoff and run the counterfactual check described in [pattern verification](../../references/pattern-verification.md). Missing proof, a changed assumption, or a different consistency decision is a model discrepancy even when the unit tests are green.

Never hide a domain change in code-only edits. Candidates prepare the next review and do not authorize implementation. Use only the controlled file-backed commands; do not hand-edit reviewed records or policy. Read [registry maintenance](../../references/registry-maintenance.md).
