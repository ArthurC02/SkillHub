---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T08:44:17.756466Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L461-469"
    excerpt_sha256: "sha256:0734baf9caef3676d4f395afb2bd899fb910ac2400f6065a172bba6611fbf131"
  - resource: "apps/platform/internal/creator/credit/refusals_test.go#L89-111"
    excerpt_sha256: "sha256:08bd42221a5031af88f93868f9c240e18b23622e85aa7a0db217cea391dbe6ac"
---
A credit statement returns at most one page of entries and says whether more remain, by asking for one entry past the page.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- With 51 entries a statement returns 50 and says more remain; with 50 it says none remain.
