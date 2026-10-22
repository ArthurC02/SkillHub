---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0128_release_declared_license.sql#L1-2"
    excerpt_sha256: "sha256:93a10e3790fed35a7542d4e26edcae6abe78996c445317f89e1b88dedb66e497"
---
publication_releases keeps check publication_releases.declared_license: declared_license text CHECK (declared_license IN ('Apache-2.0', 'MIT', 'ISC', 'BSD-2-Clause', 'BSD-3-Clause', 'GPL-2.0', 'GPL-3.0', 'LGPL-3.0', 'AGPL-3.0', 'MPL-2.0', 'Unlicense', 'CC0-1.0', 'CC-BY-4.0', 'CC-BY-SA-4.0'))

# Relations
- Contexts: [publishing](/contexts/publishing)
