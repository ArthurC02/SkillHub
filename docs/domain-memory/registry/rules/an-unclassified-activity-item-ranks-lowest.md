---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L366-366"
    excerpt_sha256: "sha256:09735b3809a6511b7802afee6065a3a4e495baa1f3a0fdca20f6656aee22d386"
  - resource: "apps/platform/internal/product/activity/limits_test.go#L52-59"
    excerpt_sha256: "sha256:1ba348214298696cfc0b041b74ace09d9e071fca02a2b72e92a5deddec8f4aec"
---
An item whose classification the feed does not know ranks below needs-attention, in-progress and recent items.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- An item classified 'something-new' sorts after a recent one, even when newer.
