---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/judging/judge.go#L118-125"
    excerpt_sha256: "sha256:59be6f055e55721da287d8349e952186618fda91203ee5a33373bf30691cc412"
  - resource: "apps/platform/internal/trial/improvement/judging/judge_test.go#L780-801"
    excerpt_sha256: "sha256:b1ba9088cc6e00ea4ed4f657725e179c2cfbc5a87cacd539090828305d60ce1a"
---
The judge is sent at most twenty criteria, and a longer list is marked as cut so no pass rests on it.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A snapshot with 25 criteria sends 20 and records the cut.
