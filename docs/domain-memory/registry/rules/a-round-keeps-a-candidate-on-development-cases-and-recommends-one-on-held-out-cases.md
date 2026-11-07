---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:32:47.518774Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/reflection/candidates.go#L14-29"
    excerpt_sha256: "sha256:29e8678b08c4e47ef92a779e552774ba7364996715a4d90a90411a2b108f124c"
  - resource: "apps/platform/internal/trial/improvement/reflection/candidates.go#L41-59"
    excerpt_sha256: "sha256:c53f24e71ad7e8d458a15eb41ad421595a23724161e75a5546112e4fa6bf8bc8"
  - resource: "apps/platform/internal/trial/improvement/reflection/candidates_test.go#L14-36"
    excerpt_sha256: "sha256:acf5214b7fad3d013ede5b8b78375f9347bf6d960002071ae2976765399b99bf"
  - resource: "apps/platform/internal/trial/improvement/reflection/candidates_test.go#L40-70"
    excerpt_sha256: "sha256:0a8ba080a01dc1a16fc216d1459dcdb746e72ad899cda50bcfc7d1301b6d369d"
---
A candidate of a round is kept when, on a development test case, it has the best rate among the candidates and beats the source version's rate there; the round recommends the kept candidate with the highest mean on the held-out cases among those at least likely improved, and recommends none while it has fewer standings than candidates or while any built candidate's batch is not settled.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Two candidates tied at the best development rate above the source are both kept; one equal to the source is not.
- While one candidate's batch is still running no candidate is recommended, even if another is likely improved.
- An unavailable candidate with no batch does not hold the recommendation back.
