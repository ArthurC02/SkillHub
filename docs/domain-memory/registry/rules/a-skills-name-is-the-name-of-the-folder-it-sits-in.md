---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-22T17:35:01.950480Z"
sources:
  - resource: "apps/platform/internal/shared/skillpkg/layout.go#L212-220"
    excerpt_sha256: "sha256:c5e8afd3b48244bda5805791b7a4cc9284b764922d0a71ea9816f2bbf8b2057e"
  - resource: "apps/platform/internal/skill/admission/service.go#L259-278"
    excerpt_sha256: "sha256:f4a820e5599866674c2a02f0032f2c05f79664e1c31755a0e6ec479f80c7fb5f"
  - resource: "apps/platform/internal/skill/admission/source.go#L108-143"
    excerpt_sha256: "sha256:070ea393713564d825c6f0eb3941dab473ba57a678dd50bc164414d215154ff5"
  - resource: "apps/platform/internal/shared/skillpkg/layout_test.go#L316-349"
    excerpt_sha256: "sha256:16642890f015580d55988def204d320b1549585534f56545008ba10781ec90a7"
  - resource: "apps/platform/internal/skill/admission/source_test.go#L329-351"
    excerpt_sha256: "sha256:8034cc6d2311cd304e4c14af024a1465b039d23633111d1eb2ed34ef021c965f"
---
A Skill found in a folder the author named is refused when its declared name is not that folder's name, so one Skill cannot be addressed by two names; where the folder name is not the author's own, because an archive's single outer folder was taken as the package root or the Skill is the whole package, the declared name is accepted as given.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A plugin entry at skills/tidy-csv declaring tidy-csv is admitted; the same package at skills/tidy-tsv is refused and told both names.
- An archive that unpacks into one folder carrying the download's name, with SKILL.md inside it, is admitted whatever that folder is called.
- A package with no name at all is refused for the missing name, not for the folder.
