---
name: domain-memory-implementation-hygiene
description: Check that a code change expresses reviewed Domain Memory boundaries without turning generic style rules into Registry facts. Use while writing or refactoring code from a handoff, when a repository check forces a change, when an external system is added or replaced, and before handing the change to review.
---

# Domain Memory implementation hygiene

Use this after Domain Memory Read or Design and before handing off an implementation or review. It checks whether the change makes reviewed ownership, invariants, and collaborations visible in code. It is a companion to the lifecycle Skills, not another Registry lifecycle.

The repository's automation owns formatting and every numeric limit. This check owns how a change meets them: whether the code that turned a check green states the domain more clearly, or only differently.

## When you are the one writing the code

The commands are run as `<python> <path>/scripts/registry_tools.py <command>`; [AGENTS.md](../../AGENTS.md) has the runtime. These seven hold for every change to source:

1. Run `quality-gates --repo-root <repo>`. The checks it lists must pass, and a check met by a change of form alone is not met: when one forces a change, remove the cause it points at. A flag kept as a keyword argument or a default is a change of form; when it chooses between kinds the domain names, including a kind the handoff says is coming, name that concept in this change. When `standard` is `none`, say so in your report and claim no standard the repository does not enforce.
2. Search for what already exists, and use it or extend it.
3. Take names from the reviewed terms. A business number, such as a threshold, a rate, or a limit, gets the domain's name and one home, and every place it appears uses that name.
4. Every rule you add or move ends with a test in the repository that fails when the rule is broken. Prove it with `counterfactual`, once for each rule: `survived` means no test protects that rule, so write the test and run it again. A comparison you run once and discard does not count. Report each rule that survived before you wrote its test.
5. An external system goes behind a Port named in the domain's terms and declared where a decision can reach it without loading the provider. Address, credentials, and status codes stay in the Adapter. A failure the domain survives is handed to the caller or recorded, never dropped.
6. A mechanical change gets the change, the tests, and a short report.
7. A migration that creates or alters a table is done when `describe-table --repo-root <repo> --migrations <dir> --table <name> --require-complete` exits 0, or `--base <ref>` in place of `--table` when older tables carry gaps of their own: the table and every column but the policy's exempt columns (`id`, `created_at` and `updated_at` unless `schema_conventions` says otherwise) say what they mean in a `COMMENT ON`; a status-like column (its name ends in one of the policy's status-like suffixes) lists its values in a CHECK or an enum, or its comment begins with one of the policy's free-text markers (`free text` by default), or another CHECK constrains it; a CHECK list on a nullable column decides whether NULL is allowed (`NOT NULL`, `col IS NULL OR` / `col IS NOT NULL AND` written into the CHECK, or another CHECK on the table that writes `col IS NULL` or `col IS NOT NULL`, because `col IN (...)` alone lets NULL through); the comment explains each listed value; each foreign key states `ON DELETE`; and no `DO` block or function builds the table's DDL at run time, unless a later migration restates the trigger the template names with a static `CREATE TRIGGER` on that table. `gaps` lists what is missing.

## Inputs

Use the compact handoff from [implementation handoff](../../references/implementation-handoff.md) when the change is material. For a routine change, read the smallest reviewed records that identify the Context, owner, and contract. If those facts are unknown, do not infer them from a convenient code shape; route a material uncertainty to Design.

## Check the boundary

Name the role of each changed area, then look for the violation that role invites:

- **Domain code** owns invariants and domain decisions. Look for a dependency on HTTP, wire formats, or provider concepts.
- **Application code** coordinates domain objects, transactions, and Context collaboration. Look for an operation that returns a transport view, or a foreign Context written around its owner.
- **An inbound adapter** authenticates, scopes, translates, and maps a result to its transport contract. Look for a domain decision or a transaction it owns.
- **An outbound adapter** translates a domain-owned Port to an external protocol. Look for a provider detail that crosses it. When the change adds, replaces, or reaches an external system, decide the boundary with [ports and adapters](../../references/ports-and-adapters.md) before writing it.
- **Persistence and generated mappings** stay behind the owning Context's established boundary. Do not add a one-implementation Repository interface to satisfy a pattern name.

A fixed public payload is named rather than left structurally anonymous; genuinely dynamic metadata stays dynamic.

## Check the expression

Rules 1 to 3 above are the check; [code expression](../../references/code-expression.md) says how to meet them. Two cases need more:

- When a repository check forced the change, search the change for each tell in *When a gate forces the change*, and report any check that could only be met in form.
- For a refactoring, keep the proof that behaviour was preserved in the repository as tests, and report each rule that had no test before the change.

## Decide the smallest response

Run `boundary-impact --registry-root <registry> --repo-root <repo> --base <branch point>` first. `unregistered-co-change` means the change touched two Contexts that have no registered collaboration, and `boundary-evidence-changed` means a file a boundary record cites changed: check whether a port, contract, or call between them changed shape, and treat it as the first case below if it did.

1. If the behavior is a material change, as [the seven-step workflow](../../references/seven-step-workflow.md) defines it, stop implementation and route it to Design.
2. If the behavior preserves reviewed facts but the code obscures a role, make the smallest local refactoring that restores it. Add no pattern, interface, layer, or folder without a protected behavior.
3. If the finding is about formatting or a limit and the code already states the domain clearly, leave it to the repository's tools. Do not record it as Domain Memory.
4. If no reviewed boundary is obscured, report no change. A long file or a familiar pattern name is not evidence by itself.

A mechanical change inside a reviewed boundary gets the change, the tests, and a short report. Send a changed domain fact to Maintain; otherwise record only the verification result in the normal delivery evidence.
