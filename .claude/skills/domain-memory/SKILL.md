---
name: domain-memory
description: "Route domain-driven development work to the smallest Domain Memory capability: read the model, design a boundary, maintain the files, or review consistency."
---

# Domain Memory

Domain Memory is a continuous development loop, not a one-time knowledge dump. The reviewed Registry guides implementation; implementation evidence updates the Registry for the next Agent.

Choose one focused capability:

Before choosing a capability in an unfamiliar repository, run the read-only
`readiness` command. It distinguishes an empty repository, a greenfield
design, a brownfield implementation, and an evidence-backed dead memory. Treat
its signals as routing evidence, not as domain facts.

See [readiness](references/readiness.md) for the evidence and output contract.

- **Read**: before coding, load the reviewed terms, Context, ownership, invariants, and existing collaboration. Use [domain-memory-read](skills/domain-memory-read/SKILL.md).
- **Design**: when a request changes a boundary, contract, event, consistency rule, or material business rule, prepare a reviewable Change Package. Use [domain-memory-design](skills/domain-memory-design/SKILL.md).
- **Maintain**: after implementation or source drift, update the smallest affected asset as a candidate and preserve evidence. Use [domain-memory-maintain](skills/domain-memory-maintain/SKILL.md).
- **Review**: before relying on domain facts or handing off work, validate the Registry, source evidence, audit chain, and proposal state. Use [domain-memory-review](skills/domain-memory-review/SKILL.md).
- **Implementation hygiene**: after Read or Design, check that code expresses the reviewed boundary without turning generic style rules into Registry facts. Use [domain-memory-implementation-hygiene](skills/domain-memory-implementation-hygiene/SKILL.md).

For a brownfield refactoring that preserves the reviewed model, use the
[brownfield refactoring fast path](references/brownfield-refactoring.md) before
opening a Change Package. It is a proof of preservation, not a shortcut around
unknown ownership or changed contracts.

Reviewed records constrain code; candidates are handoff material and do not authorize implementation. Keep all capabilities on the same file-backed Registry, policy, source map, audit chain, and Change Package schema.

Read [the seven-step workflow](references/seven-step-workflow.md) for material domain changes and [the file-backed API](references/script-api.md) for command shapes. The scripts are the controlled write boundary; Git remains the collaboration and review boundary.

A Registry that has never named an approval authority holds candidates only, and no capability can promote one. The API reference's *Preparing the approval authority* section covers the two situations: a repository whose pull requests are reviewed, and a repository where the maintainer signs instead.

Use [tactical design reasoning](references/tactical-reasoning.md) when guiding implementation. Preserve domain boundaries and invariants, then let the Agent choose the language-idiomatic tactical pattern; do not turn this Plugin into a code template library.

For a material change, pass the compact [implementation handoff](references/implementation-handoff.md) from Read or Design to the coding Agent. It carries facts and proof obligations, not code shape.

Verify any claimed tactical Pattern with [pattern verification](references/pattern-verification.md), including a counterfactual test when the change is material.

Supporting references: [evidence rules](references/evidence-rules.md), [Registry authoring](references/registry-authoring.md), [Registry schema](references/registry-schema.md), [proposal lifecycle](references/proposal-lifecycle.md), and [reliability target architecture](references/reliability-architecture.md).

Plugin portability stops at the host boundary: `plugin.json` packages Skills and optional scripts, but does not declare subagents. A host may delegate these capabilities to its own agents; otherwise run them sequentially. See [host integration](references/host-integration.md).

Distribute the complete capability set as the Plugin. Distribute one directory under `skills/` when a host accepts only a standalone Skill; the root router is not a standalone replacement for the lifecycle capabilities. See [packaging](references/packaging.md).
