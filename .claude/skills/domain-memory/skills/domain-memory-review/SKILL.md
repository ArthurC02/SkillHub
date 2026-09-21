---
name: domain-memory-review
description: Check that Domain Registry facts, source evidence, audit history, proposals, and implementation handoff remain internally consistent.
---

# Review Domain Memory

Use this before relying on a domain fact or handing work to another Agent.

1. Run `validate --registry-root <root> --repo-root <repo>`, using `--require-reviewed` when implementation depends on reviewed facts.
2. Run `verify-sources`, `verify-evidence`, and `verify-audit`.
3. Validate any Change Package and confirm its Registry revision, approvals, test attestations, and SCM evidence still match.
4. Review the implementation handoff against the tactical reasoning questions: whether the complexity is earned, invariant ownership, consistency boundary, failure behavior, dependency direction, and observable proof. Do not require a particular language construct or file layout. See [tactical design reasoning](../../references/tactical-reasoning.md).
5. Report stale, missing, invalid, ambiguous, or candidate facts as blockers or review gaps.

For a material change, confirm that the implementation handoff's proof obligations and counterfactual check have evidence, and that unresolved unknowns were not silently converted into domain facts. Read [implementation handoff](../../references/implementation-handoff.md) and [pattern verification](../../references/pattern-verification.md).

Review is read-only. Do not promote candidates, approve proposals, or repair evidence by guessing. Read [the proposal lifecycle](../../references/proposal-lifecycle.md) when reviewing a proposal.
