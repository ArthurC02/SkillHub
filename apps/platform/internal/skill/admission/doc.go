// Package ingest is the current Go package for ADR-038's proposed product domain
// 「Skill 接納與信任」: it lets creators establish source, licence, format and
// static-risk facts before a Skill is tried or delivered. Its stable Boundary
// ID/package slug remains `ingest`; ADR-038 is Proposed and does not rename it.
// It is the Trust & Supply Chain context (ADR-032 §1, requirement prefixes SKILL and SEC): the one validated path by which outside content
// becomes a Skill Version (SKILL-001, INGEST-001..010).
//
// Packages are analyzed statically only — nothing inside is executed (iron rule
// 1) — and accepted content lands as an immutable skill version plus the
// original archive in object storage (ADR-003).
//
// # What it owns
//
// One table, `skill_sources` — where a package came from and whether that origin
// still resolves (db/query-owners.yaml: skill_import.sql, plus ListSourcesToCheck,
// MarkSourceChecked, GetLineageSource and PurgeUnreferencedSkillSources). The
// skill and version rows an import produces are registry's; this context owns the
// provenance attached to them, which is why DISC-003 can say "unknown origin" and
// mean it.
//
// The more important thing it owns has no table: [Service.SaveVersion] is the
// only validated write path onto a version. A second creation path would be a
// second definition of what the platform accepts, which is why adopting an
// evaluation suggestion comes back through here (M4 PACK-002 裁定) rather than
// growing its own.
//
// # Relationships (ADR-032 §2)
//
//	ingest -> registry    synchronous write (Customer–Supplier). The validation
//	                      pipeline is here; the tables are registry's, so the row
//	                      writes go through CreateSkillFromPackage,
//	                      CreateVersionFromPackage and UpdateSummaryFromPackage.
//	                      They take this package's pgx.Tx, so the version row, the
//	                      search document and the audit event are one commit
//	                      (iron rule 9).
//	eval -> ingest        synchronous. Applying accepted suggestions builds a new
//	                      version through SaveVersion.
//	ingest -> catalog     the search projection write, injected at the composition
//	                      root rather than imported (ADR-034), the same shape
//	                      registry uses for the same table.
//	ingest -> llmclient   ACL. Index-time enrichment (ADR-013): summary, task
//	                      examples, embedding.
//	ingest -> identity    synchronous, iron rule 3.
//	ingest -> skillpkg    Shared Kernel. Zip reading, the manifest and the
//	                      validator itself; DDD-006 moved those pure helpers there
//	                      precisely so four contexts could stop importing this one
//	                      as a library.
//	identity -> ingest    inverted: PurgeWorkspace is injected into the account
//	                      purge, and must run after registry's step (see
//	                      identity/purge.go) or every source still backs a live
//	                      version and none is removed — with no error and no row
//	                      count out of place to show it.
//
// # Public face
//
// [Service.SaveVersion] and the upload/import entry points around it,
// [Result]/[UploadResult]/[NewUploadResult] for rendering one stored version the
// way every creation path does, [URLFetcher] plus [DefaultAllowedHosts] for the
// composition root, and [PurgeWorkspace]. Nothing else should be reached for from
// another context — the package-reading helpers other contexts once imported from
// here now live in skillpkg.
//
// # What is deliberately not here
//
// Not version immutability, which is registry's aggregate rule and is enforced by
// the 0005 trigger and db/query-owners.yaml, not by this package's discipline.
//
// Not the search document's field semantics: this package computes the enrichment
// because the package reader lives here, but what those fields mean to retrieval
// is catalog's, and the write is catalog's function.
//
// Not the decision to take content down. CheckSources only marks: an upstream URL
// that 404s today may be a rename, a rate limit or an outage, and the snapshot the
// platform holds stays a validated immutable fact either way (iron rule 4).
// Unpublishing is the manual takedown path, on purpose.
//
// Not execution of anything, in any form. A package is bytes and an fs.FS from
// the moment it arrives; scripts inside it are read, never run (iron rule 1,
// ADR-007).
//
// # Failure modes
//
// requireProjection refuses any path that maintains the search projection when
// the injected write is missing. Carrying on would let an import commit and
// report success while the document never appears — a skill that exists and
// cannot be found, with nothing going red.
//
// A nil LLM is a degraded mode, not a refusal: documents land `pending` and wait
// for the next import or a `reindex` backfill. Retrying is a Go decision (iron
// rule 6) and that flag is the queue it reads; there is deliberately no automatic
// schedule.
//
// URL import is allow-list only (SEC-003). The list plus a size cap is the whole
// SSRF story: redirects are re-checked against the same list and nothing internal
// is ever on it. A nil Fetcher disables URL import rather than widening it.
package ingest
