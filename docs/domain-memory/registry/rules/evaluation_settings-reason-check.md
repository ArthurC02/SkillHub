---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0116_operator_platform_settings.sql#L10-10"
    excerpt_sha256: "sha256:a70e67dd418df1edae882b62e5aeca4fdba812d382be99452a2a49d32455153c"
---
evaluation_settings keeps check evaluation_settings.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [eval](/contexts/eval)
