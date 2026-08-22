// Package registry is the current Go package for ADR-038's proposed product
// domain 「Skill 資產與版本歷史」: it lets creators create or Fork traceable Skills
// while keeping immutable versions and lineage. Its stable Boundary ID/package
// slug remains `registry`; ADR-038 is Proposed and does not rename this package.
// It owns skills and their immutable versions (ADR-002 Skill Registry & Versioning, ADR-032 §1). Two tables, and the whole package hinges
// on the fact that they follow opposite rules.
//
//	skills          mutable. summary, access_restriction, redistribution,
//	                takedown_at and deleted_at all get written after the row
//	                exists. It is the *identity* of a skill, and identity is
//	                allowed to be corrected.
//
//	skill_versions  immutable. A row is written once and never again. It is a
//	                *snapshot* of what a package was at one moment, and a
//	                snapshot that can be edited is not evidence of anything.
//
// Getting these two backwards is the failure mode this package has to guard
// against, because a caller holding both ids sees one API and two rule sets.
//
// # The Skill Version aggregate (ADR-003, iron rule 4)
//
// The invariants, each stated as what must never happen:
//
//  1. A row in skill_versions is never updated and never deleted. Improving a
//     skill means inserting the next version; nothing rewrites the previous one.
//     Adopting an evaluation suggestion is an insert (ADR-026), not an edit.
//  2. version_number is allocated by the CreateSkillVersion query itself, from
//     max(version_number)+1 over the skill. No caller passes it — NewVersion has
//     no such field, and giving it one would let two concurrent imports agree on
//     a number instead of losing on skill_versions_number_key and retrying.
//  3. content_hash, package_object_key and manifest are the snapshot. The
//     manifest column holds what the package declared, not what the skills row
//     currently says — a fork renames the skill and the version keeps the
//     original name.
//  4. license_expression and license_source travel together (ADR-021): an
//     expression without its provenance tier reads as a claim the package made
//     about itself. Both NULL is the honest answer for "unknown".
//  5. A fork copies rows, not bytes. The package object is content-addressed and
//     shared, so a fork is a new skills row plus a new skill_versions row
//     pointing at the same object, plus the lineage columns (WS-001).
//
// # Who may write
//
//	write.go        the only path that creates skill_versions rows from an
//	                imported package. It takes a skillpkg.Report rather than
//	                loose fields, so the argument that decides what is written is
//	                the validation result itself: a caller that skipped
//	                validation has nothing to pass. ingest calls these inside its
//	                own transaction (AGENTS.md "版本寫入的唯一驗證路徑").
//	registry.go     Fork, the second and last writer of skill_versions, copying
//	                an existing snapshot forward. Delete and Takedown write only
//	                skills columns — a deleted or taken-down skill keeps every
//	                version row it had.
//	restriction.go  access_restriction on skills. Mutable by design.
//
// # What actually enforces immutability
//
// Not this comment, and not the absence of an UPDATE. Two mechanisms:
//
//   - Postgres. db/migrations/0005_immutability.sql attaches enforce_immutable()
//     to skill_versions, so an UPDATE or DELETE raises restrict_violation. The
//     one hole is the account-deletion purge, which opens DELETE only inside a
//     transaction that has run SET LOCAL skillhub.purge = 'on' (0013).
//     db/tests/immutability_test.sql is the runnable proof.
//   - CI. db/query-owners.yaml declares skill_versions in its `immutable:`
//     section and `go -C tools/devctl run . automation-check` fails the build on
//     any query that updates or deletes it. The trigger catches the mistake at
//     runtime in whichever environment ran it first; this catches it on the pull
//     request that wrote it. The purge is the one named exemption there too.
//
// The other files carry no aggregate rules: diff.go reads two package objects
// out of object storage and compares them, http.go is transport.
package registry
