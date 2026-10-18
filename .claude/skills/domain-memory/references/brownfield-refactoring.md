# Brownfield refactoring fast path

Use this path for a refactoring in an implemented repository when the change preserves the reviewed Context owner, invariants, event meaning, and public contract. It keeps a mechanical boundary cleanup from becoming a new domain design exercise.

1. Run `readiness --repo-root <repo> --registry-root <registry>` and stop if it reports `dead` or a block.
2. Run `verify-sources --repo-root <repo> --source-map <registry>/source-map.json`, `verify-evidence --registry-root <registry> --repo-root <repo>`, and `verify-audit --registry-root <registry>`. Reuse a current Registry and source map; never run `init-domain-memory` over an existing directory. The command rejects non-empty output by design.
3. Read only the affected Context, its declared collaborators, and the cited invariant or contract: `get-context --id <id> --summary`, then `analyze-boundary --source-context <id> --target-context <id>` for each collaborator. Record the source locations used for the refactor.
4. Classify the change. The fast path applies only when it changes an adapter, persistence representation, tactical code shape, or injection wiring while preserving the reviewed model.
5. Implement with a focused proof: affected tests, the relevant architecture check, and a `counterfactual` when a claimed behavior would otherwise look green without proving the boundary.
6. Re-cite what the change moved or edited, so the Registry still stands on the code:
   - `boundary-impact --base <branch point>` names the Contexts the change touched and the boundary records whose evidence changed; `unregistered-co-change` or `boundary-evidence-changed` means a port, contract or call may have changed shape, which escalates.
   - `verify-evidence --path <file>` for each edited file: `shifted` passes and is re-cited in passing, `stale` needs the fact checked.
   - `relocate-evidence --from <old> --to <new>` when files or directories moved. It re-cites every citation that left `from`, and gives a Context whose `implementation_path` is `from`, under it, or ends it as the Registry stores it (`billing/orders` for `src/billing/orders`) the new path, so `boundary-impact` keeps mapping the moved code to its Context; `validate --repo-root` refuses a reviewed path left behind.
   - `recite --asset <asset> --id <id> --path <file> --lines <a>-<b> [--onto <file>]` for a citation gone `stale` where the code changed in place and the fact still holds.
   - `init-change-package --recitation <file> --requirement-id <id> --proposal-id <id> --proposer <you>` packages what the two commands printed; submit it, record its one test result, and review it as usual.
7. Run Maintain after implementation. Record a candidate only when the source evidence, owner, invariant, collaboration surface, or contract actually changed; otherwise the re-citation package is the record that the reviewed model was preserved, because it changes nothing but citations and paths.

Escalate to the seven-step workflow when the refactor is a material change as that workflow defines it, an added or split Aggregate among them. Uncertainty is an escalation trigger, not permission to assume the model is unchanged.
