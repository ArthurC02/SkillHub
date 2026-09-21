# Host integration

Domain Memory is portable because its durable state is files in the repository and its behavior is Skills plus scripts. The Plugin manifest does not define `agents` or `subagents`; do not add either field.

The host chooses execution. It may assign the four capabilities to separate agents when it supports delegation, or run them in one agent in this order:

1. `domain-memory-read` before implementation.
2. `domain-memory-design` when the change affects a boundary, contract, invariant, event, or material business rule.
3. `domain-memory-review` before handing the implementation off, and again after implementation when the result is material.
4. `domain-memory-maintain` after implementation or source drift.
5. `domain-memory-review` before relying on the maintained result.

Delegated work must use the same repository, Registry, evidence rules, and Git review boundary. A subagent may prepare a proposal or evidence, but it must not treat a candidate as reviewed or bypass the host's write and approval controls.

Host-specific role files, model settings, concurrency, and orchestration belong outside the Plugin manifest. A host adapter may map these four capability names to its own agent mechanism without changing the Registry format or the Skills.

When delegation is available, pass the implementation handoff from Read or Design to the coding Agent as the task's reviewed context. The receiving Agent may choose different language-native constructs, but it must return evidence for the handoff's proof obligations. If the host cannot carry this handoff between agents, run the capabilities sequentially in one context and label the result unverified when the handoff is lost.

The first repository event is readiness routing. Run `readiness --repo-root <repo>` before any capability that expects a Registry. An empty or greenfield repository must go through source discovery and explicit Registry initialization before Read; do not call `probe` as the first command.

The lifecycle adapter should wire these command groups to its own events:

| Host event | Capability | Commands |
| --- | --- | --- |
| Before capability selection | Router | `readiness --repo-root <repo>` |
| Empty or greenfield repository | Router / Design | `discover-sources`, explicit source choice, then `init-domain-memory`; use Design after initialization |
| Brownfield repository before implementation | Read | `probe`, `validate --require-reviewed`, then term and Context lookup |
| Dead or unusable Registry | Router / Review | Stop, report the readiness block, and recover or reconfirm sources before relying on facts |
| Before a material domain change | Design | `init-change-package`, package validation, and the seven-step workflow |
| Before implementation handoff | Review | Registry, source, evidence, audit, and Change Package validation |
| After implementation or source drift | Maintain | `verify-sources`, `verify-evidence`, `verify-audit`, then candidate or Change Package preparation |
| Before relying on the result | Review | Registry, source, evidence, audit, and Change Package validation |

The adapter must report a missing Registry in an empty or greenfield repository as a routing state, and an invalid or stale Registry as a blocking handoff state. It must not silently skip the lifecycle because a host cannot delegate or because a command has no result.

An adapter is conforming when it can show, for each lifecycle event, the command result, the Registry path used, and the Git revision when one exists. A host that cannot provide one of these fields may still run local drafts, but must label the handoff as unverified rather than presenting it as reviewed.
