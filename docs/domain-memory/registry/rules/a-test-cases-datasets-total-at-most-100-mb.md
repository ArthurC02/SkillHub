---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/design/dataset.go#L255-258"
    excerpt_sha256: "sha256:ef18ae3b573be41765ba3e4fdeec703582f0a43d6e06a82d71af57c1d952630e"
---
All of a Test Case's dataset files together may not exceed MaxTestCaseBytes; an upload past it is refused.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- At exactly the limit an upload is stored; one byte over is refused.
