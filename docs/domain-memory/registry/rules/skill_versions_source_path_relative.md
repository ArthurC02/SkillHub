---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0084_skill_version_source_path.sql#L4-8"
    excerpt_sha256: "sha256:52b6b50877519312ee85209fab4b4f3b513169eca44e223a9770759faf411481"
---
skill_versions keeps check skill_versions_source_path_relative: CONSTRAINT skill_versions_source_path_relative CHECK ( source_path = '' OR (source_path !~ '^/' AND source_path !~ '/$' AND source_path !~ '(^|/)\.\.(/|$)') )

# Relations
- Contexts: [registry](/contexts/registry)
