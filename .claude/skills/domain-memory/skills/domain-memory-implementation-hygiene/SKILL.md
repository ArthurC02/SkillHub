---
name: domain-memory-implementation-hygiene
description: Check that a code change expresses reviewed Domain Memory boundaries without turning generic style rules into Registry facts. Use while writing or refactoring code from a handoff, when a repository check forces a change, or when an external system is added or replaced; it routes a material change to Design.
---

# Domain Memory implementation hygiene

Use this after Domain Memory Read or Design and before handing off an implementation or review. It checks whether the change makes reviewed ownership, invariants, and collaborations visible in code. It is a companion to the lifecycle Skills, not another Registry lifecycle.

The repository's automation owns formatting and every numeric limit. This check owns how a change meets them: whether the code that turned a check green states the domain more clearly, or only differently.

The commands are run through the executable [running the commands](../../AGENTS.md#running-the-commands) names. The seven rules every change to source must meet are in [AGENTS.md](../../AGENTS.md#when-you-are-the-one-writing-the-code); this page is how to check a change against them.

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

Rules 1 to 3 of the seven are the check; [code expression](../../references/code-expression.md) says how to meet them. Two cases need more:

- When a repository check forced the change, search the change for each tell in *When a gate forces the change*, and report any check that could only be met in form.
- For a refactoring, keep the proof that behaviour was preserved in the repository as tests, and report each rule that had no test before the change.

## Decide the smallest response

Run `boundary-impact --registry-root <registry> --repo-root <repo> --base <branch point>` first. `unregistered-co-change` means the change touched two Contexts that have no registered collaboration, and `boundary-evidence-changed` means a file a boundary record cites changed: check whether a port, contract, or call between them changed shape, and treat it as the first case below if it did.

1. If the behavior is a material change, as [the seven-step workflow](../../references/seven-step-workflow.md) defines it, stop implementation and route it to Design.
2. If the behavior preserves reviewed facts but the code obscures a role, make the smallest local refactoring that restores it. Add no pattern, interface, layer, or folder without a protected behavior.
3. If the finding is about formatting or a limit and the code already states the domain clearly, leave it to the repository's tools. Do not record it as Domain Memory.
4. If no reviewed boundary is obscured, report no change. A long file or a familiar pattern name is not evidence by itself.

A mechanical change inside a reviewed boundary gets the change, the tests, and a short report. Send a changed domain fact to Maintain; otherwise record only the verification result in the normal delivery evidence.
