-- 0012_license_provenance: record *where* a version's license came from (ADR-021).
-- license_expression alone cannot distinguish "the author declared MIT in frontmatter"
-- from "the repository root had an MIT file and the packer carried it in", and
-- DISC-003 requires the weaker evidence to be visible as such rather than presented
-- as the skill's own declaration (curated-skill-list.md §5.3).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.

-- Written once at version-creation time, like every other column on this table, so
-- the 0005 skill_versions_immutable trigger needs no change: it freezes rows on
-- UPDATE/DELETE and this column is only ever populated by the INSERT.
ALTER TABLE skill_versions
    ADD COLUMN license_source text
        CHECK (license_source IN ('manifest', 'package-license-file', 'repo-license-file'));

COMMENT ON COLUMN skill_versions.license_source IS
    'ADR-021 provenance tier of license_expression, strongest first: manifest (author '
    'declared it in SKILL.md frontmatter), package-license-file (a LICENSE file in the '
    'package itself), repo-license-file (repository-level LICENSE carried into a package '
    'cut from a monorepo subdirectory). NULL whenever license_expression is NULL.';

-- Provenance is meaningless without a license, and a license without provenance cannot
-- be displayed under the DISC-003 rules, so the two are written together or not at all.
-- NOT VALID on purpose: versions imported before this migration legitimately have a
-- license with no recorded provenance, and back-filling a tier for them would be
-- inventing evidence. New rows are checked; the old ones stay as the facts they are.
ALTER TABLE skill_versions
    ADD CONSTRAINT skill_versions_license_source_pairing
    CHECK ((license_expression IS NULL) = (license_source IS NULL)) NOT VALID;
