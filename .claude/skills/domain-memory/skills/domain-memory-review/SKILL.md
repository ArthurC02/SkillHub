---
name: domain-memory-review
description: Check that Registry facts, source evidence, audit history, proposals and the implementation handoff still agree. Use before relying on a domain fact; it checks the Registry itself, not a branch, and is read-only.
---

# Review Domain Memory

Use this before relying on a domain fact or handing work to another Agent.

1. Run `validate --registry-root <root> --repo-root <repo>`, using `--require-reviewed` when implementation depends on reviewed facts.
2. Run `verify-sources --repo-root <repo> --source-map <root>/source-map.json`, `verify-evidence --registry-root <root> --repo-root <repo>`, and `verify-audit --registry-root <root>`.
3. Validate any Change Package and confirm its Registry revision, approvals, test attestations, and SCM evidence still match.
4. Review the implementation handoff with the [review questions](../../references/tactical-reasoning.md#review-questions). Require no particular language construct or file layout.
5. Report stale, missing, invalid, ambiguous, or candidate facts as blockers or review gaps.

For a material change, confirm that the implementation handoff's proof obligations and counterfactual check have evidence, and that unresolved unknowns were not silently converted into domain facts. See [implementation handoff](../../references/implementation-handoff.md) and [pattern verification](../../references/pattern-verification.md).

Review is read-only. Do not promote candidates, approve proposals, or repair evidence by guessing. Read [the proposal lifecycle](../../references/proposal-lifecycle.md) when reviewing a proposal.
