---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0088_bundles.sql#L56-62"
    excerpt_sha256: "sha256:9c750a2d4c166913277fdefedbcfc2b5c64a71fb89d1fd765f6676db4cd98212"
---
download_artifacts keeps check download_artifacts_one_subject: CONSTRAINT download_artifacts_one_subject CHECK ((skill_version_id IS NULL) = (plugin_name IS NOT NULL) AND (plugin_name IS NULL) = (plugin_version IS NULL))

# Relations
- Contexts: [packaging](/contexts/packaging)
