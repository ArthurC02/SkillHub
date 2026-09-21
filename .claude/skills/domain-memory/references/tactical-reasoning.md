# Tactical design reasoning

Domain Memory records the business facts that implementation must preserve. It does not prescribe a class hierarchy, directory layout, framework, or language-specific DDD template. The Agent already knows common tactical patterns; use the Registry to decide which problem the implementation must solve, then choose the smallest idiomatic design that solves it.

DDD tactical design is selective. A read-only projection, a mechanical adapter, and a simple mapping may need no domain object at all. Do not introduce an Aggregate, Entity, Value Object, Domain Service, Repository, or Event only because its name sounds architectural. A deliberate choice to keep a change as orchestration, a transaction script, or an adapter is valid when the reviewed model has no invariant or domain decision there.

## Questions before coding

Answer these from reviewed records and the requirement:

1. Which object or boundary must keep each invariant true?
2. Which state changes belong in one consistency boundary, and which facts may arrive later?
3. Which concepts need identity, and which are defined entirely by their values?
4. Which operation is a domain decision, which is orchestration, and which is persistence or transport?
5. Which Context owns each fact, and what is the narrowest interaction with another Context?
6. What must remain true after failure, retry, duplicate delivery, or partial completion?

7. Is a tactical pattern needed here, or would one add indirection without protecting a domain rule?

The answers may lead to an Aggregate, Entity, Value Object, Domain Service, Repository port, Domain Event, Policy, or another design. They may also justify no tactical pattern. Name a pattern only when it clarifies the decision; the name is not proof that the implementation is correct.

## Decision ladder

Use the smallest reasoning loop that fits the change:

1. **Classify the change.** Separate domain decisions from transport, persistence, mapping, and operational mechanics. Routine changes inside an approved boundary do not need a full tactical design record.
2. **State the forces.** Write the invariant, owner, consistency need, lifecycle, failure behavior, and collaboration constraints that make the design non-trivial.
3. **Inspect the local language.** Read nearby code and tests to learn the repository's idioms. Do not import a pattern's textbook shape when the language expresses the same behavior more clearly another way.
4. **Compare options.** Consider the smallest direct design, one design that gives the invariant an explicit owner, and an asynchronous or boundary-preserving option when relevant. Reject options by the behavior they cannot guarantee.
5. **Choose and prove.** Select the simplest option that protects the forces. Write tests at the boundary where an invalid state, duplicate, retry, stale read, or forbidden dependency would become observable.

## Implementation handoff

For a material change, record a short implementation design in the Change Package and emit the [implementation handoff](implementation-handoff.md). Define its evidence with [pattern verification](pattern-verification.md):

- the domain behavior being protected and the forces that make the choice non-trivial;
- the selected approach, which may explicitly be "no tactical pattern";
- the invariant, ownership, consistency, retry, and failure behavior it preserves;
- the rejected simpler or competing approach and the behavior it could not guarantee;
- the tests or architecture checks that would expose a boundary violation.
- the counterfactual mutation that should make the focused proof fail.

Mention a language-native construct only when choosing another construct would change the domain behavior. Do not record fixed filenames, package names, inheritance trees, framework recipes, or generated code as Domain Memory unless the repository has independently made them part of an architectural contract. A language may express the same model with different native constructs; review the behavior and dependencies, not superficial shape.

## Review questions

Review the implementation against the model:

- Can an invalid state be created through the public mutation path?
- Does a technical adapter own a domain decision that belongs to the Context?
- Does a repository or handler cross the consistency boundary without an explicit interaction?
- Are events emitted for committed business facts and safe for duplicate delivery?
- Do tests demonstrate the invariant and failure behavior rather than only the chosen type names?
- Does the selected design earn its complexity, or could a smaller design preserve the same facts?

If the answers are unclear, report a design gap and ask for a Change Package update. Do not solve uncertainty by adding a generic DDD template.
