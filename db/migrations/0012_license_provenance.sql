ALTER TABLE skill_versions
    ADD COLUMN license_source text
        CHECK (license_source IN ('manifest', 'package-license-file', 'repo-license-file'));

COMMENT ON COLUMN skill_versions.license_source IS
    'ADR-021 provenance tier of license_expression, strongest first: manifest (author '
    'declared it in SKILL.md frontmatter), package-license-file (a LICENSE file in the '
    'package itself), repo-license-file (repository-level LICENSE carried into a package '
    'cut from a monorepo subdirectory). NULL whenever license_expression is NULL.';

ALTER TABLE skill_versions
    ADD CONSTRAINT skill_versions_license_source_pairing
    CHECK ((license_expression IS NULL) = (license_source IS NULL)) NOT VALID;
