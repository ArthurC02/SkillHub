# Two evaluation-batch terms

The contract and code use baseline for two different measurements: the trials run without the Skill, and the earlier batch a comparison measures against. This proposal records each as its own reviewed term, defined by the decision that owns evaluation batches, and lists the names to avoid.

- Skill withheld: a Run that does not load the Skill under evaluation. Avoid: baseline, without skill, no skill.
- Reference batch: the batch a comparison of two batches measures against; the other side is the candidate batch. Avoid: baseline.

Once reviewed, the vocabulary-synonyms gate refuses new uses of the avoided names in contract keys and in identifiers declared in the run and eval Contexts. Today's uses are held in a baseline that may only shrink.
