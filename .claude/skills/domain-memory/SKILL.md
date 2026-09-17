---
name: domain-memory
description: "Route domain-driven development work to the smallest Domain Memory capability: read the model, design a boundary, maintain the files, or review consistency."
---

# Domain Memory

Domain Memory is a continuous development loop, not a one-time knowledge dump. The reviewed Registry guides implementation; implementation evidence updates the Registry for the next Agent.

Choose one focused capability:

- **Read**: before coding, load the reviewed terms, Context, ownership, invariants, and existing collaboration. Use [domain-memory-read](skills/domain-memory-read/SKILL.md).
- **Design**: when a request changes a boundary, contract, event, consistency rule, or material business rule, prepare a reviewable Change Package. Use [domain-memory-design](skills/domain-memory-design/SKILL.md).
- **Maintain**: after implementation or source drift, update the smallest affected asset as a candidate and preserve evidence. Use [domain-memory-maintain](skills/domain-memory-maintain/SKILL.md).
- **Review**: before relying on domain facts or handing off work, validate the Registry, source evidence, audit chain, and proposal state. Use [domain-memory-review](skills/domain-memory-review/SKILL.md).

Reviewed records constrain code; candidates are handoff material and do not authorize implementation. Keep all capabilities on the same file-backed Registry, policy, source map, audit chain, and Change Package schema.

Read [the seven-step workflow](references/seven-step-workflow.md) for material domain changes and [the file-backed API](references/script-api.md) for command shapes. The scripts are the controlled write boundary; Git remains the collaboration and review boundary.
