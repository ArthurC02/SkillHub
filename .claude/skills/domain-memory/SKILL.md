---
name: domain-memory
description: "Route domain-driven development work to the smallest Domain Memory capability: read the reviewed model before coding, extract it from a requirement or code, design a material change as a Change Package, keep the Registry current after implementation, review its consistency, or review a branch through it."
---

# Domain Memory

Domain Memory is a development loop, not a knowledge dump: the reviewed Registry guides implementation, and the implementation's evidence updates the Registry for the next Agent. Only `reviewed` records are facts; a `candidate` is handoff material and authorizes nothing.

First time here? [AGENTS.md](AGENTS.md) has the vocabulary, the common tasks with their commands, and the mistakes that force a redo.

Run `readiness --repo-root <repo>` first; [readiness](references/readiness.md) maps each of its states to the capability to open.

Then choose one capability:

- **Read** before coding, when a request uses a domain term, may cross a Context, or reads or writes a database table: [domain-memory-read](skills/domain-memory-read/SKILL.md).
- **Extract** when the Registry has never seen the source, a requirement, a codebase or a change: [domain-memory-extract](skills/domain-memory-extract/SKILL.md).
- **Design** when the change is material, as the seven-step workflow defines it, to prepare a reviewable Change Package: [domain-memory-design](skills/domain-memory-design/SKILL.md).
- **Maintain** when merged code moved or edited cited lines, or a selected source moved, to re-cite evidence and record what the model learned: [domain-memory-maintain](skills/domain-memory-maintain/SKILL.md).
- **Review** before relying on a fact, to check the Registry itself: records, evidence, audit chain and proposals: [domain-memory-review](skills/domain-memory-review/SKILL.md).
- **Review a change** when asked to review a pull request or a branch, from one `brief`, to decide whether it needs a Change Package, a recite, or nothing: [domain-memory-review-change](skills/domain-memory-review-change/SKILL.md).
- **Implementation hygiene** while writing code from a handoff, when a repository check forces a change, or when an external system is added or replaced: [domain-memory-implementation-hygiene](skills/domain-memory-implementation-hygiene/SKILL.md).

A brownfield refactoring that preserves the reviewed model takes the [brownfield refactoring fast path](references/brownfield-refactoring.md) before anyone opens a Change Package: it is a proof of preservation, not a way around unknown ownership or a changed contract.

A Registry that has never named an approval authority holds candidates only, and no capability can promote one. *Preparing the approval authority* in [script-api.md](references/script-api.md#preparing-the-approval-authority) covers it: a maintainer or reviewer who signs, and the two-person rule when the policy asks for it.

Where the rest is: [the seven-step workflow](references/seven-step-workflow.md) for a material change; [script-api.md](references/script-api.md) for every command and what it guarantees; [host integration](references/host-integration.md) when a host dispatches these capabilities to its own agents; [reliability architecture](references/reliability-architecture.md) for the trust boundaries and the write protocol behind the scripts.
