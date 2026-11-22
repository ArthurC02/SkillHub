---
type: Business Rule
status: draft
---
A Skill found in a folder the author named is refused when its declared name is not that folder's name, so one Skill cannot be addressed by two names; where the folder name is not the author's own, because an archive's single outer folder was taken as the package root or the Skill is the whole package, the declared name is accepted as given.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- A plugin entry at skills/tidy-csv declaring tidy-csv is admitted; the same package at skills/tidy-tsv is refused and told both names.
- An archive that unpacks into one folder carrying the download's name, with SKILL.md inside it, is admitted whatever that folder is called.
- A package with no name at all is refused for the missing name, not for the folder.
