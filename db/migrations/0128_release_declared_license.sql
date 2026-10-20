ALTER TABLE publication_releases
    ADD COLUMN declared_license text CHECK (declared_license IN ('Apache-2.0', 'MIT', 'ISC', 'BSD-2-Clause', 'BSD-3-Clause', 'GPL-2.0', 'GPL-3.0', 'LGPL-3.0', 'AGPL-3.0', 'MPL-2.0', 'Unlicense', 'CC0-1.0', 'CC-BY-4.0', 'CC-BY-SA-4.0'));
