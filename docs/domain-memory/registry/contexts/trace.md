---
type: Bounded Context
title: Run Trace
status: stable
subdomain: supporting
implementation_path: trial/evidence
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/doc.go#L1-2"
    excerpt_sha256: "sha256:f3d4a13d0e09f1d1cbd3eadfae53604c5e7d6a34fba0b0f47357a9648eaf91a3"
---
Owns Run trace evidence: masking, storing and reading what a Skill trial actually did.

# Requirement prefixes
- TRACE
