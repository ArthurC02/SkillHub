# Implementation handoff

The handoff is the small bridge between reviewed Domain Memory and the Agent that changes source code. It is a decision record, not a code-generation prompt. Keep it short enough to pass between agents without losing the facts that make the design safe.

## Handoff contents

Produce these sections for a material change:

```text
Domain facts:
- reviewed Contexts, terms, owners, invariants, contracts, and events

Forces:
- consistency, lifecycle, authorization, retry, failure, latency, and dependency constraints

Decision:
- the smallest design that protects those facts, including "no tactical pattern" when appropriate

Unknowns:
- unresolved facts that block implementation or require an explicit assumption

Proof obligations:
- observable tests or architecture checks that can falsify the decision

Counterfactual check:
- the essential protection to mutate, the focused test expected to fail, and the restoration result
```

Do not fill a missing fact with a likely value. Mark it unknown and stop only when the unknown affects ownership, consistency, authorization, contract compatibility, or irreversible behavior. A local implementation detail can remain an implementation choice for the coding Agent.

## Producer and consumer

- **Read** produces the reviewed facts, gaps, and tactical questions before coding.
- **Design** adds the forces, decision, alternatives, and proof obligations for a material change.
- **Coding Agent** chooses the language-native implementation after inspecting nearby code and tests. It must preserve the handoff's behavior, not copy a pattern name.
- **Maintain** compares the resulting behavior with the handoff and turns any changed fact into a candidate or new Change Package.
- **Review** checks that every proof obligation has observable evidence and that the implementation did not silently replace an unknown with an assumption.

The handoff may contain a pattern name, but a pattern name alone is never an approval or proof. Use [pattern verification](pattern-verification.md) to define the evidence. The evidence is the preserved invariant, boundary behavior, and failure behavior.

Material Change Packages declare `change_classification: "material"` and include an `implementation_design`. Its `proof_obligations` are IDs from `test-obligations.json`; the validator rejects an unknown ID. Before a material proposal becomes verified, approved, or applied, its evidence bundle records a passed `counterfactual_check` with the mutated protection, failing evidence, linked obligation, and restoration result.

## When a handoff is unnecessary

Do not create a tactical handoff for a purely mechanical rename, formatting change, isolated mapping, or implementation contained by an already reviewed boundary. Read the Registry as usual and record the normal verification result. More ceremony is not more DDD.
