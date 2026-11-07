---
type: Aggregate
status: draft
root: Candidate Evaluation
sources:
  - resource: "db/migrations/0151_candidate_evaluation.sql#L57-62"
    excerpt_sha256: "sha256:5a89e8b4a9378ac6615fad8f340f582066ffb31d7218cd58022bfacbce1471af"
  - resource: "db/migrations/0151_candidate_evaluation.sql#L67-70"
    excerpt_sha256: "sha256:7ba1141632ee754949c0276c1c1fb3b634b4af39ecf9ff385e2b649cd91f0244"
  - resource: "db/migrations/0151_candidate_evaluation.sql#L82-101"
    excerpt_sha256: "sha256:58fb168c072cf52caede67f2a82722f41fbba8c67e4fd435c7a4d86517ae29d5"
  - resource: "db/migrations/0151_candidate_evaluation.sql#L126-140"
    excerpt_sha256: "sha256:aff53fbbf99e09386af8bdc72d746d2a6412b7145d675e9ec2704cc657c03172"
  - resource: "apps/platform/internal/trial/improvement/candidate_evaluation.go#L208-242"
    excerpt_sha256: "sha256:697afeba9ce0fc55de008550ec98aea9701967606d16e8de30910da606bfbc49"
---
# Relations
- Context: [eval](/contexts/eval)

# Invariants
- Evaluating a round's candidates writes one row per candidate in reflection_candidate_batches, once: a built candidate names its held version and the batch that ran it, which the composite foreign key to evaluation_batches keeps on that version; an unavailable candidate names neither, and a batch belongs to at most one candidate.
- A candidate's batch is launched under the origin key batch-reflection/<round>/candidate/<k>, unique per workspace in evaluation_batches, so a retried evaluation returns the batch it already made.
- A round adopts at most one candidate: reflection_adoptions is keyed by the round, written once, and its foreign key to reflection_candidate_batches carries a skill_version_id that is NOT NULL, so only a built candidate can be adopted.
- A suggestion's decided_by in evaluation_suggestions is set only once it is decided.
