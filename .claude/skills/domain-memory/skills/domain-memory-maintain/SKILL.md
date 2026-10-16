---
name: domain-memory-maintain
description: Keep file-backed Domain Memory current after implementation or source changes by preserving evidence and creating governed candidate updates.
---

# Maintain Domain Memory

Use this after implementation and whenever a selected source moves.

For a brownfield refactoring that preserves the reviewed model, follow the [brownfield refactoring fast path](../../references/brownfield-refactoring.md) to decide whether a candidate is needed before creating one.

1. Run `verify-sources --repo-root <repo> --source-map <registry>/source-map.json --policy <registry>/domain-memory-policy.json`, `verify-evidence --registry-root <registry> --repo-root <repo>`, `verify-audit --registry-root <registry>`, and `boundary-impact --registry-root <registry> --repo-root <repo> --base <branch point>`, which names the Contexts the change touched together and the boundary records whose evidence changed. `<registry>` is the registry root, the directory that holds `registry/`, `source-map.json`, and the policy, not `registry/` itself.
2. Compare the implemented behavior with the reviewed terms, owner, invariants, boundaries, events, contracts, and capabilities.
3. Compare the implementation's tactical choice, including an explicit choice of no pattern, with the invariant, ownership, consistency, retry, and failure behavior it claims to preserve. Review behavior and dependencies, not a fixed language pattern. See [tactical design reasoning](../../references/tactical-reasoning.md).
   When the change touches an adapter, configuration, a client or a timeout, check it against [the assembly point](../../references/ports-and-adapters.md#at-the-assembly-point).
4. The model changed when any of these changed: a term, which Context owns a decision, an invariant or rule, a boundary or allowed dependency, an event or contract, or a capability. A change to who may decide a value, its bounds, or when it is decided is a rule change; moving a value from deployment configuration to a setting someone adjusts at run time is one. Moving or renaming code while all of these stay put is a refactor.
   If the model changed, update the smallest affected asset with `upsert-candidate`, or prepare a Change Package for a material change. For a boundary-preserving refactor, retain the focused architecture check when one exists; otherwise retain affected tests and lint as preservation evidence.
5. From the repository root, run `validate --registry-root <registry> --repo-root <repo>`, then preserve the audit trail.

For a material change, compare the implementation with its implementation handoff and run the counterfactual check described in [pattern verification](../../references/pattern-verification.md). Missing proof, a changed assumption, or a different consistency decision is a model discrepancy even when the unit tests are green.

Never hide a domain change in code-only edits. Candidates prepare the next review and do not authorize implementation. Use only the controlled file-backed commands; do not hand-edit reviewed records or policy. Read [registry maintenance](../../references/registry-maintenance.md).
