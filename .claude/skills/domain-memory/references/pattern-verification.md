# Pattern verification

Verify a DDD tactical pattern by the behavior it protects, not by its name or shape. A pattern is justified only when removing its essential protection would make a required behavior fail.

## Verification questions

For each selected approach, answer:

1. **Problem:** Which domain force requires this design?
2. **Invariant:** Which invalid state or forbidden transition can it prevent?
3. **Ownership:** Which Context and domain concept make the decision?
4. **Boundary:** Which transaction, consistency, dependency, and event rules does it preserve?
5. **Failure:** What happens on retry, duplicate delivery, timeout, partial completion, or stale data?
6. **Cost:** What complexity does it add, and why is a smaller approach insufficient?

If any answer is unknown, keep it as an open question. Do not replace it with a conventional Pattern description.

## Evidence matrix

Use the smallest evidence that can falsify the design:

| Claim | Evidence |
| --- | --- |
| An invariant is protected | Boundary and negative tests reject the invalid state. |
| A Context owns the decision | Ownership and dependency checks show callers cannot write around it. |
| Consistency is correct | Transaction, contract, or event tests observe the promised timing and result. |
| Retries are safe | The same command or event is applied twice and produces the required result once. |
| Failure behavior is correct | A controlled failure leaves the documented state and recovery path. |
| Complexity is justified | Removing the essential protection makes one of the above tests fail. |

The last row is the counterfactual check. It must mutate the essential protection, run the focused test, observe a failure, and restore the implementation. A passing test without this check proves only that the test exists.

## Review outcome

Accept the pattern when the evidence demonstrates the required behavior and the design is the smallest option that provides it. Request a redesign when the evidence checks names, inheritance, file layout, or framework usage without showing a domain consequence. Record "no tactical pattern" when the behavior remains correct without one.
