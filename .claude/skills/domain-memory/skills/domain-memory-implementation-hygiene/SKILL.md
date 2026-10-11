---
name: domain-memory-implementation-hygiene
description: Check that a code change expresses reviewed Domain Memory boundaries without turning generic style rules into Registry facts. Use while writing or refactoring code from a handoff, when a repository check forces a change, when an external system is added or replaced, and before handing the change to review.
---

# Domain Memory implementation hygiene

Use this after Domain Memory Read or Design and before handing off an implementation or review. It checks whether the change makes reviewed ownership, invariants, and collaborations visible in code. It is a companion to the lifecycle Skills, not another Registry lifecycle.

The repository's automation owns formatting and every numeric limit. This check owns how a change meets them: whether the code that turned a check green states the domain more clearly, or only differently.

## When you are the one writing the code

The commands below are in [registry_tools.py](../../scripts/registry_tools.py): run `python3 <that file> <command>` from any directory, with Python 3.10 or later and `git`, and nothing to install. These six hold for every change to source:

1. Run `quality-gates --repo-root <repo>`. The checks it lists must pass, and a check met by a change of form alone is not met: when one forces a change, remove the cause it points at. A flag kept as a keyword argument or a default is a change of form; when it chooses between kinds the domain names, including a kind the handoff says is coming, name that concept in this change. When `standard` is `none`, say so in your report and claim no standard the repository does not enforce.
2. Search for what already exists, and use it or extend it.
3. Take names from the reviewed terms. A business number, such as a threshold, a rate, or a limit, gets the domain's name and one home, and every place it appears uses that name.
4. Every rule you add or move ends with a test in the repository that fails when the rule is broken. Prove it with `counterfactual`, once for each rule: `survived` means no test protects that rule, so write the test and run it again. A comparison you run once and discard does not count. Report each rule that survived before you wrote its test.
5. An external system goes behind a Port named in the domain's terms and declared where a decision can reach it without loading the provider. Address, credentials, and status codes stay in the Adapter. A failure the domain survives is handed to the caller or recorded, never dropped.
6. A mechanical change gets the change, the tests, and a short report.

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

Work through [code expression](../../references/code-expression.md):

- Find the repository's own checks first; they must pass. When it has none, report that, and claim no standard the repository does not enforce.
- Search for what already exists before writing it again.
- Take identifiers and the names of business numbers from the reviewed vocabulary.
- When a repository check forced the change, search the change for each tell in *When a gate forces the change*, and report any check that could only be met in form.
- For a refactoring, keep the proof that behaviour was preserved in the repository as tests, and report each rule that had no test before the change.

## Decide the smallest response

1. If the behavior changes an owner, invariant, contract, event, or consistency rule, stop implementation and route it to Design.
2. If the behavior preserves reviewed facts but the code obscures a role, make the smallest local refactoring that restores it. Add no pattern, interface, layer, or folder without a protected behavior.
3. If the finding is about formatting or a limit and the code already states the domain clearly, leave it to the repository's tools. Do not record it as Domain Memory.
4. If no reviewed boundary is obscured, report no change. A long file or a familiar pattern name is not evidence by itself.

A mechanical change inside a reviewed boundary gets the change, the tests, and a short report. Send a changed domain fact to Maintain; otherwise record only the verification result in the normal delivery evidence.
