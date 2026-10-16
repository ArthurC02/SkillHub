---
type: Bounded Context
title: Evaluation Batches
status: stable
subdomain: core
implementation_path: trial/batch
verified:
  - by: "human:ArthurC"
    at: "2026-10-11T18:23:06.041403Z"
sources:
  - resource: "apps/platform/internal/trial/batch/doc.go#L1-3"
    excerpt_sha256: "sha256:02bde1e2e9957c0079a8304181f31e437c3e80a056a4dc874b400f33f0cabc6a"
  - resource: "docs/adr/ADR-009-evaluation-verdicts-and-judge-trust.md#L104-104"
    excerpt_sha256: "sha256:1d20da8f4fdd83214a82079afecabba38bed668a1673d0a25d0dcdad2d694efd"
---
Runs one Skill Version against chosen Test Cases a set number of times, answers their questions as each Test Case's simulated user, and reports pass rates with intervals, with the Skill withheld and against a reference batch.
