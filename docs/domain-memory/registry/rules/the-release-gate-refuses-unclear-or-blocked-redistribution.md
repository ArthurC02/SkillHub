---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:50:56.766681Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/publication.go#L88-106"
    excerpt_sha256: "sha256:fe4ff21dc505e19f5bf61dc5f83804fb65d21385798e9e027898c1a8a57962f9"
  - resource: "apps/platform/internal/skill/publishing/publication_test.go#L52-84"
    excerpt_sha256: "sha256:ed6a9eb53391cd5d36eea65f4d78d3b5b25912af48f072c62fe4ab25f63f559c"
---
A release is refused unless the Skill's redistribution is allowed, or self_supplied or generated with the author's rights statement; a license hold, a blocked licence and an unconfirmed licence are refused whatever the statement.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Self-supplied content without the statement is refused as rights_not_attested; an unconfirmed licence is refused as license_unknown even with it.
