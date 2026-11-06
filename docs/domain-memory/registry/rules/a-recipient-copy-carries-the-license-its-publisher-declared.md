---
type: Business Rule
status: draft
owner: skill-packaging
---
When a Skill Version records no license of its own and its publication release declares one, the copy a recipient acquires records that license in skillhub-manifest.json with the source tier release-declaration and the disclosure license-from-release-declaration, and its INSTALL.md says the publisher declared it and no one has checked it; SKILL.md is copied byte for byte, a license the version records itself always wins and is not disclosed, and the declaration never decides redistribution.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A generated Skill whose version has no license, released with MIT declared: the acquired manifest records MIT from release-declaration and discloses it.
- A version whose SKILL.md declares Apache-2.0, released with MIT declared: the manifest records Apache-2.0 from manifest and no release-declaration disclosure.
- Neither the version nor the release has a license: expression and source tier are both null.
