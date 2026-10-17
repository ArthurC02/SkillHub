---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T05:40:48.955499Z"
sources:
  - resource: "apps/platform/internal/trial/execution/halt/source.go#L67-79"
    excerpt_sha256: "sha256:195381d2a6330a0aed57776d7082b2dcf2dfaa4bc23b3a31f3f649e18934ce45"
  - resource: "apps/platform/internal/trial/execution/halt/source.go#L30-32"
    excerpt_sha256: "sha256:5e01be527679065645ddff212d2088f1de11330822655e740e3ced773b8839ca"
  - resource: "apps/platform/internal/trial/execution/halt/ledger.go#L155-178"
    excerpt_sha256: "sha256:7fe3f0a99544febb757133b6e56ca4c06b86bd25bbbce0c4ff6c7d7919431c26"
  - resource: "apps/platform/internal/trial/execution/halt/halt_test.go#L125-156"
    excerpt_sha256: "sha256:4d9105ab721ce45876126ed209faf56628235a601e4143b65dd6a51c5e1fa94c"
---
A dispatch halt declared as a P1 incident outranks a capacity pause on the same target: a later capacity pause replaces neither its source, its reason nor its declarer, and only an operator lifts it. A capacity pause lifts itself after two consecutive clear reconciler rounds.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- The orphan threshold trips while a P1 holds the pool: the halt stays a P1 with the responder's reason.
- A P1 arrives while a capacity pause holds a node: the halt becomes the P1 and names the responder.
- Leaks stay below the threshold for two rounds: the capacity pause lifts, while a P1 on the same target stays.
