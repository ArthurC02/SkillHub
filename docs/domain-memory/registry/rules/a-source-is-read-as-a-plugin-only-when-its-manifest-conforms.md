---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-22T18:20:04.957643Z"
sources:
  - resource: "apps/platform/internal/shared/skillpkg/discover.go#L116-163"
    excerpt_sha256: "sha256:d8d50ceddffb6378abe42c450fd8e1a6bfbd5d53d382291a8ebcf2d180f51aa2"
  - resource: "apps/platform/internal/shared/skillpkg/discover.go#L180-207"
    excerpt_sha256: "sha256:547dcae3a0fedc01ecfb7e6bae0b57744be914240ba0921169961fff2b39c7fd"
  - resource: "apps/platform/internal/shared/skillpkg/discover_test.go#L274-316"
    excerpt_sha256: "sha256:700f19fbd1d4256c1d8d69895665e1cc733c4f6e942252fcc8e367c8d23839dc"
  - resource: "apps/platform/internal/shared/skillpkg/discover_test.go#L318-350"
    excerpt_sha256: "sha256:1f8cb224c9c1c4e53c70b4c91fab4206b7029f8e80a6ef74b2fcab9dd7cc3b64"
---
A source is read as an Agent Plugin only when the plugin.json in its root declares the specification's canonical schema identifier and then conforms to it: a name of 1 to 64 lowercase alphanumerics, hyphens and periods whose first and last characters are alphanumeric and which holds no consecutive hyphen or period, and an author, where present, that is an object carrying only name, email and url, each a string. A manifest that declares another specification, declares none, or is not valid JSON is disclosed and the source is read as a directory tree instead; a manifest that declares this specification and breaks it refuses the whole source, names the field, and leaves its components undiscovered.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A manifest whose author carries twitter is refused, the refusal names twitter, and none of the skills under skills/ are discovered.
- A manifest declaring another vendor's schema identifier is disclosed at info level and its skills are imported as a directory tree.
- An extensions field that is not an object is disclosed and ignored, and the plugin still loads.
