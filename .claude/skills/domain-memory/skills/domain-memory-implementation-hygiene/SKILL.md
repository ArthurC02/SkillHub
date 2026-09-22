---
name: domain-memory-implementation-hygiene
description: Check that a code change expresses reviewed Domain Memory boundaries without turning generic style rules into Registry facts.
---

# Domain Memory implementation hygiene

Use this after Domain Memory Read or Design and before handing off an implementation or review. It checks whether the change makes reviewed ownership, invariants, and collaborations visible in code. It is a companion to the lifecycle Skills, not another Registry lifecycle.

Do not use it for generic formatting, naming, line counts, or broad style review. Existing repository automation owns those concerns.

## Inputs

Use the compact handoff from [implementation handoff](../../references/implementation-handoff.md) when the change is material. For a routine change, read the smallest reviewed records that identify the Context, owner, and contract. If those facts are unknown, do not infer them from a convenient code shape; route a material uncertainty to Design.

## Check the boundary

For each changed area, name its role before recommending code:

- Domain code owns invariants and domain decisions. It does not depend on HTTP, wire formats, or provider-specific concepts.
- Application code coordinates domain objects, transactions, and Context collaboration. It preserves the owner and consistency boundary from the handoff.
- An inbound adapter authenticates and scopes input, translates it to application language, and maps a result to its transport contract. It does not own domain decisions or transactions.
- An outbound adapter translates a domain-owned Port to an external protocol, SDK, configuration, or error vocabulary. Provider details stop there.
- Persistence and generated mappings stay behind the owning Context's established boundary; do not add a one-implementation Repository interface merely to satisfy a pattern name.

Check only concerns that can violate those roles: a technical adapter deciding domain policy, an application or domain operation returning a transport view, a foreign Context bypassing its owner, a provider detail crossing its adapter, or a fixed public payload left structurally anonymous when a named model would clarify the contract. Keep genuinely dynamic metadata and conditional payloads dynamic.

## Decide the smallest response

1. If the behavior changes an owner, invariant, contract, event, or consistency rule, stop implementation and route it to Design.
2. If the behavior preserves reviewed facts but the code obscures a role, make the smallest local refactoring that restores the boundary. Do not add patterns, interfaces, layers, or folders without a protected behavior.
3. If the finding is mechanical, use the repository's formatter, linter, test, or review process. Do not record it as Domain Memory.
4. If no reviewed boundary is obscured, report no change. A long file or a familiar pattern name is not evidence by itself.

For a source-changing refactor, prove preserved behavior with the focused check and the repository's required gates. Send a changed domain fact to Maintain; otherwise record only the verification result in the normal delivery evidence.
