---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:20:09.677572Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_root.go#L245-252"
    excerpt_sha256: "sha256:5e5ae3ce700c5eb00bc29679a23590e392323582726cf8280826288f01a3b638"
  - resource: "apps/platform/internal/skill/library/skill_root_test.go#L45-76"
    excerpt_sha256: "sha256:6c94b12b81fa4403fbd53cd8ba46c138138af1ba5ed4813393ab00a0cf5bbde4"
---
The owner may undo a takedown the owner made, within 30 days of it; a takedown an operator made, or one whose origin is unknown, only an operator lifts, at any time; restoring a Skill that is not taken down is refused.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- An owner's takedown exactly 30 days old is restored; 30 days and a second is refused; an operator's takedown an hour old is refused to the owner and lifted by an operator.
