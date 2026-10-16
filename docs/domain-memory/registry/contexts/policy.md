---
type: Bounded Context
title: "Policy & Usage"
status: stable
subdomain: supporting
implementation_path: product/entitlements
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/doc.go#L1-3"
    excerpt_sha256: "sha256:6eb739ad40ee094e307dfb76b9411efb2f868a072f1b58690e4882a81ae3ecac"
---
Owns what a Workspace may consume and how long what it produces is kept: run and generation allowances, and download artifact retention.

# Requirement prefixes
- PDM
- NFR
