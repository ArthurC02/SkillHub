---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:32:47.518774Z"
sources:
  - resource: "db/migrations/0151_candidate_evaluation.sql#L126-140"
    excerpt_sha256: "sha256:aff53fbbf99e09386af8bdc72d746d2a6412b7145d675e9ec2704cc657c03172"
  - resource: "apps/platform/internal/trial/improvement/candidate_evaluation.go#L208-242"
    excerpt_sha256: "sha256:697afeba9ce0fc55de008550ec98aea9701967606d16e8de30910da606bfbc49"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/candidate_evaluation_integration_test.go#L222-237"
    excerpt_sha256: "sha256:2d1c8d8438117bc2049361f6b7e109951b9f00c53bb7a0afb03461a8e5637401"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/candidate_evaluation_integration_test.go#L402-430"
    excerpt_sha256: "sha256:d6109295ddcdadadf5abdf888e6018aca3f9a54b17e033ac3f380699cc8bc183"
---
A round adopts at most one of its candidates and only one that was built; adopting it again answers the same adoption, adopting another is refused with 409, and the promotion is written before the adoption so a lost adoption row is healed by adopting again.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Adopting candidate 0 twice answers 200 both times; adopting candidate 1 afterwards answers 409.
- When the adoption row is lost after the promotion, adopting again records it without a second promotion.
