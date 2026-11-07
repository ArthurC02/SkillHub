---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0151_candidate_evaluation.sql#L67-70"
    excerpt_sha256: "sha256:7ba1141632ee754949c0276c1c1fb3b634b4af39ecf9ff385e2b649cd91f0244"
  - resource: "apps/platform/internal/trial/improvement/candidate_evaluation.go#L91-125"
    excerpt_sha256: "sha256:4f110cd8fc6308215f5d940fe79534fb0ee7761a160a1be3828feea2660dd376"
  - resource: "apps/platform/internal/trial/improvement/reflection/evaluate.go#L119-127"
    excerpt_sha256: "sha256:d738dfcdfe77bd451ae8c6630effc48eb32aa448b7098cbd8a21b74af1af67b1"
---
Asking for a round's candidates to be evaluated accepts each candidate's pending suggestions in the name of the user who asked, and leaves the rejected ones out of its version, recorded as skipped; a candidate with only rejected suggestions is not built and is unavailable.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Evaluating a round of two candidates with one pending suggestion each records both as accepted by the user who clicked.
- With candidate 1's only suggestion rejected, candidate 0 is built, candidate 1 is unavailable, and its record lists that suggestion as skipped while it stays rejected.
