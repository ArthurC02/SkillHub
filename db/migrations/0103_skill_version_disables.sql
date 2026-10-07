CREATE TABLE skill_version_disables (
    skill_version_id uuid PRIMARY KEY REFERENCES skill_versions (id) ON DELETE CASCADE,
    disabled_at timestamptz NOT NULL DEFAULT now()
);
