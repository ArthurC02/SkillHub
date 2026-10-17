---
name: domain-memory-extract
description: Extract DDD information into the Domain Registry from a requirement, from existing code, or from a requirement change or refactoring, recording only the domain facts and their evidence.
---

# Extract Domain Memory

Use this when the Registry should learn the domain from a source it has not read yet: a requirement, a codebase with no or little Registry, or a change whose domain effect is not yet recorded.

1. Run `readiness --repo-root <repo>`. When no Registry exists, start one with `init-domain-memory`, choosing its flags as [the workflow](../../references/seven-step-workflow.md) describes. Candidates may cite agent-asserted sources; only promotion to reviewed needs a developer to have confirmed them with `confirm-sources`.
2. Pick the entry that matches the source and follow it in [extraction](../../references/extraction.md):
   - **Requirement**: from intended behavior to terms, owner, rules, events and collaborations.
   - **Code**: from executable behavior to the model it implements, language first and boundaries second.
   - **Change**: from a requirement change, diff or refactoring to the records it adds, changes or retires.
3. Decide each element by its DDD definition, not by the technique that surfaced it. Event Storming, Domain Storytelling, Example Mapping, code reading and `change-coupling` are all valid ways to find candidates; none of them is recorded.
4. Write each finding with `upsert-candidate`, its fields as [the Registry schema](../../references/registry-schema.md) lists them for its asset, and evidence built by `cite --show`, passing each citation object whole, then run `validate`. `retract-candidate` withdraws a candidate that turns out wrong. To find the test that proves a rule, break the protection with `counterfactual` under a broad test command, such as the module's whole suite with `-v`, and read `failing_tests`: a search for the error's name misses a test that asserts its effect instead, so "no test" is claimed only after a counterfactual survives. A record states what the domain is. How it was found, which technique suggested it, and how confident the extraction felt stay out of the Registry.
5. Hand the candidates to the Review capability, `domain-memory-review`. Extraction never promotes its own findings.

When a reviewed Registry exists for the same code, extract into a scratch root of its own instead of the reviewed one: `init-domain-memory --output <scratch>`, extract there, score it with `compare-registry --gold-root <reviewed> --candidate-root <scratch>`, then bring what it found into the reviewed Registry as candidates with `harvest-candidates --registry-root <reviewed> --from-root <scratch>`, which skips copies of reviewed records. `init` makes only an empty skeleton with no policy; `init-domain-memory` is the entry point.
