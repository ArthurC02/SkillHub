# Seven-step domain change workflow

Start with `readiness --repo-root <repo>` and use the result only to choose the smallest next action, as [readiness](readiness.md) describes. It is a read-only filesystem observation; it never selects sources or changes the Registry.

Use this workflow for a material change. A change is material when it changes an owner, a boundary, an invariant, a public or event contract, an event, a consistency rule or a material business rule, a regulated rule and an irreversible behaviour among them, or when it adds a cross-Context dependency; this is the one definition the other pages refer to. A routine implementation contained within one approved owner does not need the full workflow, but it still reads the Registry before coding and checks whether the implementation changed the domain documents afterward.

A brownfield refactoring that preserves the reviewed model takes the [brownfield refactoring fast path](brownfield-refactoring.md), which escalates back here.

The workflow is continuous: the Registry is the input to implementation and the implementation is evidence for the next Registry revision. A code change that leaves a changed owner, term, invariant, boundary, event, contract, or capability undocumented is not complete.

## Before Step 0: Map repository sources

Run `discover-sources --repo-root <repo>` without writing output. Present the candidate paths to the developer and ask which paths should be trusted as Domain Memory sources and where the Registry should live inside the repository. Do not choose a default directory. Once the developer has explicitly chosen both, run `init-domain-memory --repo-root <repo> --output <chosen-path> --source <chosen-source> --storage-mode tracked --data-classification internal --review-mode local-draft-only --source-authority <developer>`, giving `--source` once for every selected source. `tracked` keeps the Registry in Git; `internal` suits a repository whose code is not public; `local-draft-only` holds candidates until `amend-policy` names who approves; `--source-authority` is the person who chose the sources. Without a person to ask, select the repository instructions and the directories that hold the code you are asked to extract, record nothing reviewed, and leave `confirm-sources` to the developer. Read every selected repository-instruction path before reading other groups. The resulting source map is a deterministic inventory; it cannot establish domain ownership or rule truth.

## Step 0: Normalize the requirement

Run `init-change-package --output <package-root> --requirement-id <id> --proposal-id <id>`, then complete [requirement-normalization.json](../templates/requirement-normalization.json). Record the intended business outcome, observable acceptance criteria, candidate terms, unknowns, and risk flags. Flag `cross_context`, `public_contract`, `regulated_rule`, `sensitive_data`, `financial_decision`, and `irreversible_change` when applicable. Discovery may continue with unknowns; implementation may not assume their answers.

## Step 1: Discover the primary Context

For each candidate term, run `resolve-terms --query <text>`. Then run `get-context --id <id>` for the selected primary Context. Record the cited asset IDs and source locations. If a term has more than one valid definition, retain the ambiguity in the requirement rather than choosing one. No match is a knowledge gap, and so is a match whose `match_kind` is `definition`. A `synonym` match names the preferred term to use instead.

## Step 2: Discover adjacent Contexts

For every external fact, identify its owner and the Context that consumes it. Run `analyze-boundary --source-context <id> --target-context <id>` before proposing a dependency. Check existing interactions and contracts before proposing a new one. `no_registered_collaboration` is a proposal trigger, not permission for a direct write, and `policy_only` means a dependency is allowed but no collaboration is recorded yet. An implementation package, database table, or endpoint name cannot establish business ownership by itself.

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

Copy [test-obligations.json](../templates/test-obligations.json). Map every acceptance criterion, invariant, contract promise, and material failure mode to an observable assertion. An obligation's `source_type` names what it is derived from and must be `acceptance-criterion`, `rule`, `contract` or `invariant`; the kind of check it runs belongs in `level`, where aggregate, integration, architecture and security checks appear only when the proposal changes them. Every acceptance criterion, and every rule and contract the proposal names, needs an obligation carrying its id. When one check proves several of them, list the others in that obligation's `also_covers` as `{"source_type": ..., "source_id": ...}` entries rather than repeating the obligation, so `record-test-result` runs the check once. An `also_covers` entry must name an acceptance criterion of the requirement or a rule or contract the proposal lists, or an invariant; any other id is refused by name, since it would cover nothing. A green implementation test alone does not prove a changed invariant.

## Step 7: Prepare, verify, and review the package

Create an [evidence bundle](../templates/evidence-bundle.json) and [Draft PR description](../templates/draft-pr.md). Include the requirement, proposal, registry revision, approvals, executed checks, failures, and residual risks. Run `validate-change-package --package-root <path> --registry-root <root>` before presenting the package. Run `ship-package` only when the package is verified and Registry records must change; it attests the signed commit, finalizes and applies. A Skill can prepare this package; repository write, PR creation, merge, and deployment remain subject to the user's authorization and the repository's own controls.

## Harvesting proven rules from existing code

`harvest --spec <file> [--through verified]` runs the whole loop below from one spec, one entry per rule, and stops before the signed commit; the steps are spelled out here for a rule that needs a hand on one of them. When a step fails after the package is written, running the same spec again resumes from where it stopped without writing the unchanged records again, and `discard-harvest --package-root <package>` undoes a harvest that will not be finished.

When reviewed rules are drawn from code that already enforces them rather than from a new requirement, start from the Contexts with the fewest reviewed rules in `coverage`'s `rules_by_context`. For each rule, cite the code that enforces it and, with `cite --function`, the test that proves it; record it with `upsert-candidate` and promote it into one package. Its `check` runs only that test, with `-v` for Go. Then show that every rule can fail before anything is submitted, once per rule:

```sh
DM="python3 <path>/registry_tools.py"
$DM counterfactual --repo-root . --file <enforcing file> --find '<protection>' --replace '<broken protection>' --test-command '<check>' --cwd <directory> --obligation-id <obligation>
```

Several rules at once go through `counterfactual --suite <json>`, one entry per rule with its `obligation_id`, one intact run per test command. Continue only when every verdict is `killed`. `harvest --spec` keeps each whole output and, with the rule's check and the spec's ownership, boundary and design decisions, writes the criteria, obligations, proof obligations, sources and `counterfactual_check` list; it writes nothing while a rule's counterfactual is not killed. A rule whose mutation survives is either untested or not the rule it claims: write the test, or leave the rule out and say why in `rejected_alternatives`.

After `submit-proposal`, `record-test-result` for each obligation, `record-approval` and `verify-proposal`, the `git-signed-commit` verifier needs three more steps. They take no decisions, but they must run in this order, because the attestation names the signed commit that carries the verified package; `ship-package` attests the signed commit, finalizes the proposal and applies it, in that order, stopping at the first refusal:

```sh
P=<package directory>; R=<registry root>
git add "$P" && git commit -S -m "docs(domain-memory): propose <package>"
$DM ship-package --package-root "$P" --registry-root "$R" --repo-root . --commit "$(git rev-parse HEAD)"
git add "$P" "$R" && git commit -S -m "docs(domain-memory): apply <package>"
git pull --no-rebase && git push
```

Upstream work comes in with a merge, never a rebase: a rebase rewrites both signed commits, and the attestation would name a commit that no longer carries the package. The signatures stay with the person or host whose key the policy authorizes. When `ship-package` refuses the proposal because its base revision is stale, run `rebase-proposal`, record the approvals again, `verify-proposal`, and sign a new commit holding the package for `ship-package`: the old approvals and the old signed commit name the previous revision.

A rule already reviewed, such as one from before test obligations were recorded, is re-proven the same way after `promote-candidate --amend --repo-root . --cite-function <test file>:<test name>` puts it into the draft with the citation of its test and names the package it supersedes. A rule that a dependency gate enforces rather than a test, such as a forbidden import between Contexts, is proven by an obligation with `level: architecture` whose `check` runs the gate; its counterfactual adds a blank import of the forbidden package, `_ "<package>"`, which still builds, so the gate is what fails.

## After implementation: reflect the model

Run `verify-evidence` and `verify-audit`, then compare the observable behavior with the reviewed records used in Steps 1–5. If the model is unchanged, record that the implementation preserved it in the normal review material. If it changed, add a candidate with `upsert-candidate` or create a new Change Package; never hide the difference in a code-only change. A candidate is the handoff to the next review, not a new constraint until an authorized reviewer promotes it.

## Stop conditions

Do not present an implementation as ready when a required term is ambiguous, a source conflict has no decision, the owner is unknown, a contract is breaking without an approved migration, or an approval-required change lacks its approval. State the gap and produce the discovery or proposal material that a reviewer needs.
