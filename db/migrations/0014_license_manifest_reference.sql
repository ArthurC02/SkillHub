-- 0014_license_manifest_reference: allow the `manifest-referenced-file` provenance
-- tier (ADR-021 待決策 #1, now decided).
--
-- A frontmatter `license` value is sometimes a *pointer* to a file rather than a
-- declaration — npm's established `SEE LICENSE IN <filename>` convention, and the
-- `Complete terms in LICENSE.txt` that `anthropics/skills` writes. Recording that
-- string verbatim as the license expression lost the Apache-2.0 those packages
-- plainly state. Resolving the pointer recovers the fact, but the evidence is not
-- the same as an author typing `license: Apache-2.0`: the author named a file, and
-- the file's text answered. It therefore gets its own tier rather than being folded
-- into `manifest`.
--
-- Ranked between `manifest` and `package-license-file`: the author chose this file
-- for this package, which is stronger than merely finding a LICENSE beside the
-- content, but the expression is read from text rather than declared outright.
--
-- Applied migrations are immutable: fix forward with a new file, never edit 0012.

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
