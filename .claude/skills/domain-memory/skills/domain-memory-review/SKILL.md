---
name: domain-memory-review
description: Check that Domain Registry facts, source evidence, audit history, proposals, and implementation handoff remain internally consistent.
---

# Review Domain Memory

Use this before relying on a domain fact or handing work to another Agent.

1. Run `validate --registry-root <root> --repo-root <repo>`, using `--require-reviewed` when implementation depends on reviewed facts.
2. Run `verify-sources`, `verify-evidence`, and `verify-audit`.
3. Validate any Change Package and confirm its Registry revision, approvals, test attestations, and SCM evidence still match.
4. Report stale, missing, invalid, ambiguous, or candidate facts as blockers or review gaps.

Review is read-only. Do not promote candidates, approve proposals, or repair evidence by guessing. Read [the proposal lifecycle](../../references/proposal-lifecycle.md) when reviewing a proposal.
