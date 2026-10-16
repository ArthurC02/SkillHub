---
type: Bounded Context
title: Product Analytics
status: stable
subdomain: supporting
implementation_path: product/learning
verified:
  - by: "human:ArthurC"
    at: "2026-09-22T10:27:24.111820Z"
sources:
  - resource: "apps/platform/internal/product/learning/doc.go#L1-3"
    excerpt_sha256: "sha256:b18edd553af433b43f4db8b80daa21b66eda5127524b33baa60db013d30bf208"
---
Owns product-journey measurement: the funnel events and beta feedback reports that let the product learn without becoming a second source of domain truth.

# Requirement prefixes
- O11Y
- PDM
