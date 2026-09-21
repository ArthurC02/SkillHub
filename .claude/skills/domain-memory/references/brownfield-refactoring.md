# Brownfield refactoring fast path

Use this path for a refactoring in an implemented repository when the change
preserves the reviewed Context owner, invariants, event meaning, and public
contract. It prevents a mechanical boundary cleanup from becoming a new domain
design exercise.

1. Run `readiness --repo-root <repo> --registry-root <registry>` and stop if it
   reports `dead` or a block.
2. Run `verify-sources`, `verify-evidence`, and `verify-audit`. Reuse a current
   Registry and source map; never run `init-domain-memory` over an existing
   directory. The command rejects non-empty output by design.
3. Read only the affected Context, its declared collaborators, and the cited
   invariant or contract. Record the source locations used for the refactor.
4. Classify the change. The fast path applies only when it changes an adapter,
   persistence representation, tactical code shape, or injection wiring while
   preserving the reviewed model.
5. Implement with a focused proof: affected tests, the relevant architecture
   check, and a counterfactual when a claimed behavior would otherwise look
   green without proving the boundary.
6. Run Maintain after implementation. Record a candidate only when the source
   evidence, owner, invariant, collaboration surface, or contract actually
   changed; otherwise record that the reviewed model was preserved.

Escalate to the seven-step workflow when the refactor adds or splits an
Aggregate, changes a consistency boundary, changes an event or public
contract, introduces a new cross-Context dependency, or changes a material
business rule. Uncertainty is an escalation trigger, not permission to assume
the model is unchanged.
