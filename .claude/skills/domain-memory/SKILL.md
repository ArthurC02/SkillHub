---
name: domain-memory
description: Build and maintain a reviewable Domain Registry, or analyze a proposed change that crosses Bounded Contexts. Use for domain vocabulary, ownership, invariants, aggregate, event, contract, or cross-context design; do not use for routine changes contained within one owner.
---

# Domain Memory

Use this skill to build reviewed domain assets or turn a domain-significant request into an implementable, reviewable change. It separates evidence, design choices, approvals, and unresolved decisions before files are selected or modified.

Start from the business behavior, not a table, endpoint, event name, or package. Establish the ubiquitous terms in the request, the owner of each fact, the invariant that must hold, and the reason a boundary must be crossed. A package boundary is evidence of an architectural boundary; it does not by itself define the business model.

Domain facts are not inferred from package names, database tables, event strings, or token extraction. A repository scan can create evidence candidates and coverage gaps; it cannot create approved vocabulary, a business owner, an invariant, or an Aggregate boundary. Mark these as candidates until an authorized reviewer confirms them.

Choose the narrowest collaboration surface that preserves the invariant. A direct call, injected read-only fact, synchronous owner operation, published domain event, or a contract change can each be valid. Do not prescribe events or APIs merely because more than one context is involved. State the required consistency, idempotency, authorization, failure, and observability behavior when they affect the choice.

Derive verification from acceptance criteria and actual behavior branches. A test should demonstrate the stated invariant, contract promise, or observable outcome; it must not merely mirror the proposed implementation.

## Script routing

Use `scripts/registry_tools.py` as the first source for Registry facts. Do not replace these commands with a repository keyword search when the Registry is available.

## Probe before anything else

At the start of work in a repository, run `probe --repo-root <repo> --registry-root <root>` first. It answers three questions in one cheap call and writes nothing: whether a Domain Memory exists here, who confirmed its sources, and whether those sources have moved since. It compares the tracked object of every selected path and the working-tree status of those paths, and only re-reads file contents when that comparison cannot settle the question. Exit code 0 means current, 1 means it needs attention, 2 means no Domain Memory exists at that location.

A probe reporting `none` is not permission to initialize. It ends the automatic part of the work: the developer decides whether this repository gets a Domain Memory, and where. A probe reporting `stale` names the sources that moved, and those are the ones to re-establish.

## Init checkpoint

Before creating a Domain Memory directory or writing any file under it, run `discover-sources --repo-root <repo>` without `--output`. Then ask the developer to choose both the source paths and the repository-relative destination directory. Present the discovered paths as suggestions and allow paths outside that list when they exist inside the repository. Do not default to `domain-memory/`, `docs/`, or any other location.

Proceed only after the developer explicitly identifies both choices, the storage mode, data classification, review mode, source authority, include patterns, and exclusions. Initialize with `init-domain-memory --repo-root <repo> --output <chosen-path> --source <chosen-source> --storage-mode <mode> --data-classification <classification> --review-mode <mode> --source-authority <authority> [--include <glob>] [--exclude <glob>]` once, repeating `--source` for every selected path. A second run against the same destination fails, because the destination must not already hold a Registry. The command writes the Registry, developer-confirmed `source-map.json`, and policy only under the chosen destination. Its resource limits apply before the source map is accepted.

| Situation | Required Script action | What to do with the result |
| --- | --- | --- |
| Beginning work in any repository | `probe --repo-root <repo> --registry-root <root>` | `none` hands the decision to the developer. `stale` names the sources to re-establish. `current` also reports whether a developer confirmed the selection or an agent asserted it. |
| First use in a repository | `discover-sources --repo-root <repo>` | Present discovered candidates and wait for the developer to choose source paths and destination. |
| Start work in a repository with a Registry | `validate --registry-root <root> --repo-root <repo>` and `validate-policy --policy <root>/domain-memory-policy.json` when present | Stop reliance on malformed data; use `--require-reviewed` before an implementation may depend on Registry facts. |
| Developer confirmed source paths and destination | `init-domain-memory --repo-root <repo> --output <root> --source <path>` | Create only an empty candidate Registry and developer-confirmed source map at that destination. |
| Requirement contains a domain term | `resolve-terms --query <text> [--context <id>]` | Record every match. No match is a knowledge gap, not permission to invent a definition. |
| A Context is named or selected | `get-context --id <id>` | Use its aggregates, rules, contracts, and interactions to identify ownership. A missing Context stops a domain-significant implementation. |
| A specific asset ID is cited | `get-record --asset <asset> --id <id>` | Use the returned evidence and status; do not assume an ID exists. |
| A known external Context is involved | `analyze-boundary --source-context <id> --target-context <id>` | Reuse registered collaboration where possible. `no_registered_collaboration` requires a proposal, never a direct cross-context write. |
| The target asset is uncertain | `lookup --asset <asset> --query <text>` and `coverage` | Treat an empty result or coverage gap as discovery work. |
| Check developer-confirmed sources before relying on their evidence | `verify-sources --repo-root <repo> --source-map <root>/source-map.json --policy <root>/domain-memory-policy.json` | A stale, unverified, or limit-invalid map is a review trigger, not proof that the Domain Memory is wrong. |
| Check citations or audit integrity | `verify-evidence --registry-root <root> --repo-root <repo>` and `verify-audit --registry-root <root>` | Treat stale citations or an invalid audit chain as a blocker for reviewed facts. Legacy string citations remain readable but are not verified. |
| Curating new or changed evidence | `upsert-candidate --registry-root <root> --repo-root <repo> --asset <asset> --record-file <file>` | Preflight the complete Registry, then apply a recoverable transaction. Never edit a reviewed record in place. |
| Proposing a domain-significant change | `init-change-package --output <path>` then `validate-change-package` | Fill Requirement, Proposal, Test Obligations, Evidence Bundle, and Draft PR before implementation. |
| Sending a Proposal to review | `submit-proposal --package-root <path> --registry-root <root> --repo-root <repo>` | Capture the Registry digest and observed current commit before human review. The proposing identity cannot approve its own Proposal. |
| Verify planned checks | `verify-proposal --package-root <path> --registry-root <root> --repo-root <repo>` | Promote only a submitted package with one passing digest-backed test attestation for every obligation and, when configured, an allowlisted command profile. |
| Completing human review | `record-approval`, `verify-proposal`, then `finalize-proposal --registry-root <root> --repo-root <repo>` | Finalize only a verified package whose SCM attestation, required roles, and Registry revision still match. |
| Import external review proof | `verify-scm-attestation --attestation <file> --package-root <path>` | Verify an already-collected SCM artifact binds to the exact Proposal and Registry revision. It does not call a hosting-provider API. |
| Applying Registry changes from an approved proposal | `apply-approved-updates --package-root <path> --registry-root <root> --repo-root <repo>` | The command requires an approved current proposal, validates the whole Registry in staging, and refuses to overwrite reviewed records. |

Use these resources only when they fit the task:

- Read [registry authoring](references/registry-authoring.md) and [registry schema](references/registry-schema.md) only when creating or curating Registry records.
- Read the [file-backed Domain Memory API](references/script-api.md) only when a command needs its input or output shape.
- Read the [seven-step workflow](references/seven-step-workflow.md) for a domain-significant change; read [proposal lifecycle](references/proposal-lifecycle.md) only before submitting, approving, or applying its Proposal.
- Read [registry maintenance](references/registry-maintenance.md) only when cited sources changed; read [evidence rules](references/evidence-rules.md) only when sources conflict, are untracked, or a decision status is unclear.
- Read the [reliability architecture](references/reliability-architecture.md) before changing Registry storage, evidence, approvals, test attestations, or write behavior.
- Copy [change-impact-report.md](templates/change-impact-report.md) only when the requested output is an impact report rather than a Change Package.

The report is a design artifact. It does not create approval requirements or authorize changes beyond the user's request.
