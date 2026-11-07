---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/reflection/candidates.go#L14-29"
    excerpt_sha256: "sha256:29e8678b08c4e47ef92a779e552774ba7364996715a4d90a90411a2b108f124c"
  - resource: "apps/platform/internal/trial/improvement/reflection/candidates.go#L41-59"
    excerpt_sha256: "sha256:c53f24e71ad7e8d458a15eb41ad421595a23724161e75a5546112e4fa6bf8bc8"
---
A candidate of a round is kept when, on a development test case, it has the best rate among the candidates and beats the source version's rate there; the round recommends the kept candidate with the highest mean on the held-out cases among those at least likely improved, and recommends none while it has fewer standings than candidates or while any built candidate's batch is not settled.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Two candidates tied at the best development rate above the source are both kept; one equal to the source is not.
- While one candidate's batch is still running no candidate is recommended, even if another is likely improved.
- An unavailable candidate with no batch does not hold the recommendation back.
