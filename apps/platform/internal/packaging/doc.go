// Package packaging turns an immutable Skill Version into a downloadable
// package (ADR-032 §1 Packaging & Distribution, requirement prefix PACK;
// PACK-001..005, ADR-012, ADR-027).
//
// It is the inverse of import: it reads the bytes a version was stored with,
// filters them against an allow-list, applies the target profile, adds the
// platform's own three files, and hands the result to the same validator an
// import goes through. A package this refuses is a package the platform would
// not accept back (PACK-009).
//
// Nothing here executes anything (iron rules 1 and 2). There is no os/exec, no
// unpacking to disk and no interpretation of package content; bytes flow as
// []byte and fs.FS and nowhere else. Packaging therefore runs in the control
// plane and needs no sandbox, for the same reason M3's evaluation does.
//
// It also creates and modifies no Skill Version (iron rule 4): re-packaging is
// another Download Artifact row, never an edit of one.
//
// # What it owns
//
// `download_artifacts` and `download_records`, plus the `artifacts` rows whose
// kind is `download_package` — that table is the one place two contexts share a
// table, and db/query-owners.yaml splits it per query: run's outputs are run's,
// download packages are this context's. All three are declared immutable
// (ADR-027): a repackage is a new row, and "who downloaded what, when" is a
// record, not editable state.
//
// It also owns the packaging profiles read from disk. They are data rather than
// compiled-in because `support_status` and the install paths are reviewed text,
// and a copy of them in Go would be a second truth nobody re-reviews. A
// deployment with no profile directory has no targets and says so; it does not
// fall back to a hard-coded set.
//
// # Relationships (ADR-032 §2)
//
//	packaging -> policy     synchronous query (Customer–Supplier): may a Download
//	                        Artifact be created at all. policy decides, packaging
//	                        refuses its own request when the answer is "no
//	                        retention configured".
//	packaging -> testlab    synchronous. The curated Test Cases that travel with a
//	                        package, read through testlab's queries and decoded
//	                        with its Decode* functions rather than encoding/json.
//	packaging -> identity   synchronous, iron rule 3. Existence is private: a
//	                        version that is not the caller's answers 404, not 403
//	                        (WS-006).
//	packaging -> skillpkg   Shared Kernel — and the validator here is the same
//	                        one import runs, which is the entire PACK-009 claim.
//	objreconcile            reaches [MarkArtifactPurged] through a function
//	                        injected by cmd/worker, not an import: a generic
//	                        scanner importing a context would be the layering
//	                        upside down (ADR-033 clearance path 4). The scanner
//	                        reports the difference; the owner applies it.
//	analytics               no edge. apiserver wraps DownloadContent with
//	                        analytics' DownloadStartedOn middleware, so "somebody
//	                        pressed download" is measured without this package
//	                        knowing measurement exists.
//
// The provenance a manifest quotes — the version and its lineage, the import
// source, the applied suggestions — is read through queries db/query-owners.yaml
// assigns to registry, ingest and eval. Every such read is declared there; the
// writes stay with their owners.
//
// # Public face
//
// [Service.Plan] answers "may this be packaged, and what would come out", and
// the create call answers from the same function — a preview that says yes and a
// create that refuses cannot both happen. [Service] and [Handler] are for the
// composition root, [LoadProfiles] and [Profiles] for cmd/api, and
// [MarkArtifactPurged] is the single injected write objreconcile is given.
// [Manifest] and its parts are exported because the manifest's consumer list has
// a member outside this repository — the person unzipping the package — but the
// contract is contracts/packaging/download-manifest.schema.json, and these
// structs are one implementation of it rather than the other way round.
//
// # What is deliberately not here
//
// Not the licensing facts. `skills.access_restriction` is a hold catalog's
// operator route sets; `skills.redistribution` is a property of the content that
// arrives with it. This package reads both and refuses on either, and it must
// not treat one as the other: reading the hold as the redistribution test would
// mean "nobody objected, so it may be copied", which is the direction ADR-021
// §5.3 records a real misjudgement in. Both fail closed — an unrecognised
// restriction code still restricts, and any redistribution value that is not
// exactly `allowed` blocks, so a value added to the column tomorrow does not
// release content today.
//
// Not a signature and not an endorsement. ADR-027 decision 3 keeps the MVP
// manifest plaintext and unsigned; the two hashes answer "are these the bytes
// that were built" and "is this the content that was packaged", and neither
// answers "does Skill Hub vouch for this".
//
// Not the audit trail. A successful download writes a download record (WS-004,
// what the owner reads) and an audit event (CORE-008, what compliance keeps) in
// one transaction, deliberately as two rows with different retention and
// visibility — see internal/audit.
//
// Not a user's own files. Uploaded Dataset bytes are never packaged and there is
// no checkbox to make them: a checkbox looks like respecting a choice while
// handing a licensing judgement to the person least equipped to make it, about
// files that may belong to their employer (packaging-design §5.1).
//
// # Failure modes
//
// Artifact creation is disabled until a deployment configures retention.
// PDM-006's proposed duration is intentionally not a schema or code default —
// that would turn a proposal into a production fact.
//
// build validates what it PRODUCED, not what it read. The source version passed
// validation at import, but packaging adds files and a profile may add
// frontmatter, so validating the source would make PACK-009 a check of the wrong
// bytes.
//
// [PackagerVersion] is part of the idempotency key and is recorded in every
// manifest and every row, because content_hash reproduces within one packager
// version and is deliberately not promised across them (ADR-027 decision 2).
// Bump it whenever the produced bytes could change for unchanged input.
package packaging
