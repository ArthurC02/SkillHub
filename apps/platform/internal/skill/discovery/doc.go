// Package catalog is the current Go package for ADR-038's proposed product
// domain 「Skill 探索」: it lets creators find candidate Skills and understand
// their match and limits. Its stable Boundary ID/package slug remains `catalog`;
// ADR-038 is Proposed, so this does not rename the package, its owner facts or its path.
// It owns skill discovery (ADR-002/ADR-032 §1 Catalog & Discovery, requirement prefix DISC). Hybrid retrieval: pgvector similarity ranks,
// Postgres FTS only widens the candidate set (ADR-013 定案調整 3, measured in
// golden-query-set.md §3.7). The Python LLM service provides embeddings and
// match-reason generation; Go owns policy, auth, state and retry (iron rule 6).
//
// # What it owns
//
// One table, `search_documents` — the projection, not the skills. Everything a
// search result is *about* lives in registry's `skills` and `skill_versions`;
// what lives here is the retrieval-facing shape of it: the enriched summary, the
// task example sentences, the tags and the embedding computed from them
// (db/query-owners.yaml, search.sql plus GetSkillEnrichment). It is the manual
// CQRS read model ADR-032 §4 says is enough, which is why nothing here is the
// answer to "what is true about this skill".
//
// The display vocabularies in tier.go and trust.go are owned here too, and they
// are display policy: how much review a result has had (Tier), how sure the
// platform is of its origin (SourceTrust), what its licence says
// (LicenseStatus, Redistribution). NFR-001 forbids collapsing them into one
// "safe" badge, so they are three axes and always shown as three.
//
// # Relationships (ADR-032 §2)
//
//	catalog -> registry     synchronous owner reads for Skill, latest Version
//	                        and runtime compatibility; only Registry DTOs cross.
//	catalog -> registry     synchronous write (Customer–Supplier). The operator
//	                        hold writes one column of registry's `skills` through
//	                        registry.SetAccessRestriction, which takes this
//	                        package's transaction so the lock, the write and the
//	                        audit event stay one commit. The reason codes, the
//	                        sentences, the routes and the authorization check stay
//	                        here — see restriction.go for why splitting it that
//	                        way keeps one place that knows which codes exist.
//	catalog -> analytics    synchronous, fire-and-forget, never able to fail a
//	                        search (ADR-029 決策 1).
//	catalog -> identity     synchronous; workspace scope never comes from the
//	                        request (iron rule 3).
//	catalog -> llmclient    ACL. Embeddings and match reasons; the LLM service's
//	                        wire types stop at that package.
//	catalog -> skillpkg     Shared Kernel. Package content is re-scanned on read
//	                        rather than stored.
//	ingest -> catalog       source provenance is injected as a narrow read at the
//	                        composition root; catalog does not import ingest.
//	registry, ingest        write this table without importing this package. The
//	                        three functions in projection.go are injected at the
//	                        composition root (ADR-034): `catalog -> registry`
//	                        already exists, so `registry -> catalog` would be a
//	                        compile-time cycle, and ingest uses the same shape so
//	                        the difference does not read as if it meant something.
//
// No outbox event is published or consumed. The projection is maintained inside
// the writer's transaction (INGEST-009) rather than by reacting to an event,
// because a skill that exists and cannot be found is not an acceptable window.
//
// # Public face
//
// For other contexts, the public surface is the three projection writes:
// [IndexSkill], [IndexSkillEnriched] and [RemoveSkillFromIndex]. Each takes the
// caller's pgx transaction plus catalog-owned data and never opens a transaction
// of its own — that is the point, not an implementation detail. The enrichment
// worklist is a pool-backed [Service.PendingEnrichments] read. [Service] and
// [Handler] are for the composition root.
//
// Callers that hold an injected copy must fail closed, and both do:
// registry.requireProjection and ingest.requireProjection refuse to start a
// write at all when the function is missing. The alternative failure is the bad
// one — the fork or the takedown commits, the caller is told it worked, and the
// index quietly disagrees with the registry.
//
// # What is deliberately not here
//
// Not `skills` or `skill_versions`: catalog reads their owner DTOs and writes
// exactly one column through Registry's owner API.
//
// Not the download gate. LicenseStatusConfirmed means "a human verified this
// licence", not "this may be redistributed" — the second question is
// `skills.redistribution`, and the four locks that answer it are packaging's
// (ADR-012, ADR-027 decision 4). A caller that reads a Tier or a LicenseStatus
// as permission to release bytes has misread all three axes.
//
// Not stored package facts. Risk findings, licence provenance and the file tree
// are recomputed from the stored package on every detail read, because
// skillpkg.Validate is the single definition of what the platform discloses and
// a persisted copy would be a second one that drifts. Reading a package is
// static analysis; nothing in it is executed (iron rule 1).
//
// Not `skill_runtime_compatibility`; Registry owns and exposes that read fact.
package catalog
