# Seven-step domain change workflow

Start with `readiness --repo-root <repo>`. Use the result only to choose the
smallest next action: discover sources for an empty repository, design for a
greenfield repository, read and maintain for a brownfield repository, and
recover or ask the developer for new sources when the result is dead. The
command is a read-only filesystem observation; it never selects sources or
changes the Registry.

Use this workflow for a change that affects a domain model, crosses a Bounded Context, changes a public or event contract, or changes a material business rule. A routine implementation contained within one approved owner does not need the full workflow, but it still reads the Registry before coding and checks whether the implementation changed the domain documents afterward.

For a brownfield refactoring that preserves the reviewed model, use the
[brownfield refactoring fast path](brownfield-refactoring.md). It requires
evidence that the model was preserved and escalates back here whenever an
owner, invariant, collaboration surface, or contract changes.

The workflow is continuous: the Registry is the input to implementation and the implementation is evidence for the next Registry revision. Do not treat a completed code change as complete while it leaves a changed owner, term, invariant, boundary, event, contract, or capability undocumented.

## Before Step 0: Map repository sources

Run `discover-sources --repo-root <repo>` without writing output. Present the candidate paths to the developer and ask which paths should be trusted as Domain Memory sources and where the Registry should live inside the repository. Do not choose a default directory. Once the developer has explicitly chosen both, run `init-domain-memory --repo-root <repo> --output <chosen-path> --source <chosen-source>` for every selected source. Read every selected repository-instruction path before reading other groups. The resulting source map is a deterministic inventory; it cannot establish domain ownership or rule truth.

## Step 0: Normalize the requirement

Run `init-change-package --output <package-root>`, then complete [requirement-normalization.json](../templates/requirement-normalization.json). Record the intended business outcome, observable acceptance criteria, candidate terms, unknowns, and risk flags. Flag `cross_context`, `public_contract`, `regulated_rule`, `sensitive_data`, `financial_decision`, and `irreversible_change` when applicable. Discovery may continue with unknowns; implementation may not assume their answers.

## Step 1: Discover the primary Context

For each candidate term, run `resolve-terms --query <text>`. Then run `get-context --id <id>` for the selected primary Context. Record the cited asset IDs and source locations. If a term has more than one valid definition, retain the ambiguity in the requirement rather than choosing one. No match is a knowledge gap.

## Step 2: Discover adjacent Contexts

For every external fact, identify its owner and the Context that consumes it. Run `analyze-boundary --source-context <id> --target-context <id>` before proposing a dependency. Check existing interactions and contracts before proposing a new one. `no_registered_collaboration` is a proposal trigger, not permission for a direct write. An implementation package, database table, or endpoint name cannot establish business ownership by itself.

## Step 3: Decide the boundary

Complete [domain-change-proposal.json](../templates/domain-change-proposal.json). State the owner Context, invariant, allowed collaboration surface, consistency behavior, and prohibited dependencies. Run `validate-change-package --package-root <path> --registry-root <root>` after every material edit. Stop for review when a change adds or splits an Aggregate, changes consistency, writes across Contexts, or changes a regulated rule.

Before implementation, decide whether tactical design is needed at all. If it is, derive the smallest approach from the domain forces, record why it protects the invariant and consistency boundary, which alternatives were rejected, and how tests will expose a violation. Let the Agent choose the language-idiomatic construct; the Change Package must explain behavior and ownership, not prescribe a class, package, or framework template. See [tactical design reasoning](tactical-reasoning.md).

| Need | Collaboration shape |
| --- | --- |
| The owner must decide now and the caller needs the result now | A narrow synchronous owner operation or contract. |
| The caller needs a fact but must not own or mutate it | An injected read model or facts interface. |
| Another Context reacts after a committed business occurrence | A domain event and idempotent consumer. |
| An external client must observe or invoke the behavior | A versioned public contract. |

If the decision changes consistency, specify the observable delay, retry behavior, and idempotency key in the Proposal.

## Step 4: Decide the event

Reuse an existing event when it already expresses the committed business fact. Otherwise propose an event with its producer, consumers, version, payload classification, delivery expectation, and compatibility result. Do not name a database mutation or technical command as a domain event.

## Step 5: Decide the contract

For an API or event contract, record producer, consumers, version, compatibility policy, data classification, timeout or retry behavior where applicable, and breaking-change result. A contract change remains a proposal until its required review is recorded.

## Step 6: Derive test obligations

Copy [test-obligations.json](../templates/test-obligations.json). Map every acceptance criterion, invariant, contract promise, and material failure mode to an observable assertion. An obligation's `source_type` names what it is derived from and must be `acceptance-criterion`, `rule`, `contract` or `invariant`; the kind of check it runs belongs in `level`, where aggregate, integration, architecture and security checks appear only when the proposal changes them. Every acceptance criterion needs its own obligation carrying that criterion's id. A green implementation test alone does not prove a changed invariant.

## Step 7: Prepare, verify, and review the package

Create an [evidence bundle](../templates/evidence-bundle.json) and [Draft PR description](../templates/draft-pr.md). Include the requirement, proposal, registry revision, approvals, executed checks, failures, and residual risks. Run `validate-change-package --package-root <path> --registry-root <root>` before presenting the package. Run `apply-approved-updates` only when the package is approved and Registry records must change. A Skill can prepare this package; repository write, PR creation, merge, and deployment remain subject to the user's authorization and the repository's own controls.

## After implementation: reflect the model

Run `verify-evidence` and `verify-audit`, then compare the observable behavior with the reviewed records used in Steps 1–5. If the model is unchanged, record that the implementation preserved it in the normal review material. If it changed, add a candidate with `upsert-candidate` or create a new Change Package; never hide the difference in a code-only change. A candidate is the handoff to the next review, not a new constraint until an authorized reviewer promotes it.

## Stop conditions

Do not present an implementation as ready when a required term is ambiguous, a source conflict has no decision, the owner is unknown, a contract is breaking without an approved migration, or an approval-required change lacks its approval. State the gap and produce the discovery or proposal material that a reviewer needs.
