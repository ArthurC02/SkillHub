# Draft PR: a recipient copy carries the license its publisher declared

## Business intent

A generated Skill has no license in its own files; its author declares one when releasing it. Someone who acquires it now sees that declaration in the copy instead of an unknown license.

## Domain impact

packaging gains the rule a-recipient-copy-carries-the-license-its-publisher-declared. The interaction packaging-publishing-recipient-package now hands packaging the current release's declared license with the version to package; its citation moves with the call. Redistribution, the catalog gate and the single-Skill release checks are unchanged.

## Implementation handoff

publishing passes the current release's declared license to the recipient package hook. packaging fills a missing version license with it under the source tier release-declaration, discloses license-from-release-declaration in the manifest and adds a License section to INSTALL.md. SKILL.md is copied byte for byte. The manifest contract gains the enum value and moves to 1.3. Rejected: writing the license into SKILL.md frontmatter (overturns the generator leaving it blank and the byte-for-byte copy), and only pointing readers to the publication page (the declaration would not travel with the bytes). Bundle releases record no declaration, so a plugin's license still comes from its members.

## Proposal and approvals

release-declared-license-2026-11-06, one developer approval; the direction was ruled by the Optimization session as arbiter.

## Contract impact

contracts/packaging/download-manifest.schema.json: license.source_tier gains release-declaration; schema_version 1.3.

## Verification

rule-a-recipient-copy-carries-the-license-its-publisher-declared maps to the delivery unit table; acquired-copy-carries-the-declaration maps to the apiserver integration test against the database.

## Residual risks

The declaration is the publisher's word: no one has checked it, and it is not in the package bytes, so it never becomes a basis for redistribution.
