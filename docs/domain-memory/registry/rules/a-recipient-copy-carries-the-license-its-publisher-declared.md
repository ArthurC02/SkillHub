---
type: Business Rule
status: stable
owner: skill-packaging
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T00:12:22.295278Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/release_license.go#L10-17"
    excerpt_sha256: "sha256:0b03ed0c32e24ceef6132cb5adc2383c6c6fd540e6624c674501dc31e7e3b1ba"
  - resource: "apps/platform/internal/skill/delivery/release_license.go#L26-36"
    excerpt_sha256: "sha256:2d284175e8d5f7e2d537339186519e26765d68abd90945314e8bace0aa564f1e"
  - resource: "apps/platform/internal/skill/delivery/release_license_test.go#L8-46"
    excerpt_sha256: "sha256:fbd14e18e1bb9a51d278478bd34777bfa2cc51b41000c4378efa24c5d685885f"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/release_license_integration_test.go#L10-47"
    excerpt_sha256: "sha256:75cc7222f2b7080d60bf4d68041d0fbef63f613ffff53627e891cfcc5561a213"
---
When a Skill Version records no license of its own and its publication release declares one, the copy a recipient acquires records that license in skillhub-manifest.json with the source tier release-declaration and the disclosure license-from-release-declaration, and its INSTALL.md says the publisher declared it and no one has checked it; SKILL.md is copied byte for byte, a license the version records itself always wins and is not disclosed, and the declaration never decides redistribution.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A generated Skill whose version has no license, released with MIT declared: the acquired manifest records MIT from release-declaration and discloses it.
- A version whose SKILL.md declares Apache-2.0, released with MIT declared: the manifest records Apache-2.0 from manifest and no release-declaration disclosure.
- Neither the version nor the release has a license: expression and source tier are both null.
