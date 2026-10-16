---
name: domain-memory-extract
description: Extract DDD information into the Domain Registry from a requirement, from existing code, or from a requirement change or refactoring, recording only the domain facts and their evidence.
---

# Extract Domain Memory

Use this when the Registry should learn the domain from a source it has not read yet: a requirement, a codebase with no or little Registry, or a change whose domain effect is not yet recorded.

1. Run `readiness --repo-root <repo>`, then confirm the sources with `discover-sources` and `confirm-sources` when the source map is missing. Extraction cites only confirmed sources.
2. Pick the entry that matches the source and follow it in [extraction](../../references/extraction.md):
   - **Requirement**: from intended behavior to terms, owner, rules, events and collaborations.
   - **Code**: from executable behavior to the model it implements, language first and boundaries second.
   - **Change**: from a requirement change, diff or refactoring to the records it adds, changes or retires.
3. Decide each element by its DDD definition, not by the technique that surfaced it. Event Storming, Domain Storytelling, Example Mapping, code reading and `change-coupling` are all valid ways to find candidates; none of them is recorded.
4. Write each finding with `upsert-candidate` and evidence built by `cite`, then run `validate`. A record states what the domain is. How it was found, which technique suggested it, and how confident the extraction felt stay out of the Registry.
5. Hand the candidates to the Review capability, `domain-memory-review`. Extraction never promotes its own findings.

When a reviewed Registry exists for the same code, `compare-registry` scores an extraction against it.
