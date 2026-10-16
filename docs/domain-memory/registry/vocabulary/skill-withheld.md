---
type: Term
title: Skill withheld
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-11T18:30:00.343897Z"
sources:
  - resource: "docs/adr/ADR-009-evaluation-verdicts-and-judge-trust.md#L106-106"
    excerpt_sha256: "sha256:40fefc08b1e5050078f2fb1354d08863a75f0f9dc5448044d1a21752cf20f23d"
---
A Run that does not load the Skill under evaluation, so it measures the same Test Case without that Skill; an evaluation batch can run one beside every trial and compare the two. Not the same thing as a Reference batch.

# Relations
- Contexts: [run](/contexts/run)
- Contexts: [batch](/contexts/batch)
- Not same as: [reference-batch](/vocabulary/reference-batch)

# Synonyms
- baseline
- without skill
- no skill
