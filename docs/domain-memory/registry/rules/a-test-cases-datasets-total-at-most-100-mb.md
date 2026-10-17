---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L255-258"
    excerpt_sha256: "sha256:ef18ae3b573be41765ba3e4fdeec703582f0a43d6e06a82d71af57c1d952630e"
  - resource: "apps/platform/internal/trial/design/dataset_characterization_test.go#L182-201"
    excerpt_sha256: "sha256:351d26f8aa7d63c465e5e20b926ed3ecaa696e3627b9a837315958d34d276ded"
---
All of a Test Case's dataset files together may not exceed MaxTestCaseBytes; an upload past it is refused.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- At exactly the limit an upload is stored; one byte over is refused.
