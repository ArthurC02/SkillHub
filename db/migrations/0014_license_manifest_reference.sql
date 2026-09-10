ALTER TABLE skill_versions
    DROP CONSTRAINT IF EXISTS skill_versions_license_source_check;

ALTER TABLE skill_versions
    ADD CONSTRAINT skill_versions_license_source_check
    CHECK (license_source IN (
        'manifest', 'manifest-referenced-file', 'package-license-file', 'repo-license-file'
    ));

COMMENT ON COLUMN skill_versions.license_source IS
    'ADR-021 provenance tier of license_expression, strongest first: manifest (author '
    'declared it in SKILL.md frontmatter), manifest-referenced-file (frontmatter pointed '
    'at a package file, e.g. "SEE LICENSE IN LICENSE.txt", and that file''s text was '
    'recognised), package-license-file (a LICENSE file in the package itself), '
    'repo-license-file (repository-level LICENSE carried into a package cut from a '
    'monorepo subdirectory). NULL whenever license_expression is NULL.';
