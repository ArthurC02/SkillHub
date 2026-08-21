// Package analytics writes the product analytics events of the core funnel
// (02:O11Y-004, ADR-029) — the fifth data class, and the only one that is
// explicitly not a source of truth.
//
// What that means in practice, because it is the whole design:
//
//   - Any funnel segment an existing domain table can answer is answered there.
//     Runs come from `runs`, "was it helpful" from evaluations.feedback_helpful,
//     "was the advice taken" from evaluation_suggestions.decision, "did they
//     download" from download_records. Only four segments have no table behind
//     them, and those four are the only events this package emits. A fifth one
//     has to explain first why no domain table answers it (ADR-029 決策 1).
//   - No free text, ever. Not masked free text — none. Each event has its own
//     constructor in analytics.go, and that signature is the attribute
//     whitelist: an attribute nobody declared cannot be passed, so it cannot be
//     stored. Iron rule 11's ban on secrets in "分析事件" is met by structure
//     rather than by a masking pass, because unlike a Trace there is nothing
//     here to mask.
//   - The search query's words are not recorded. Its length and a coarse script
//     bucket are. A query can carry personal data, and the only channel that may
//     see the words is BETA-003's, which has explicit consent (ADR-029 決策 2).
//   - The session identifier is not the ADR-020 session token, nor a hash of it,
//     nor anything derivable from it. That is credential material. This is an
//     unrelated random cookie value that cannot be resolved back to a sessions
//     row, and before login it is the only linkage that exists (ADR-029 決策 4).
//
// Nothing is collected at all until a deployment sets a retention period. That is
// not caution for its own sake: NFR-002 requires a retention value to exist before
// a data class starts accumulating, ADR-029 決策 5 proposes 180 days and that
// proposal is not ratified, and this is the one data class that can still be built
// in the right order because it has not started yet.
//
// # What it owns
//
// Two tables: `analytics_events` and `feedback_reports`. db/queries/beta.sql is
// this context's file, but the file is not the boundary — CountQuotaRuns and
// RunInWorkspace read run's rows, GetWorkspaceCreatedAt and
// GetIdentityProviderIDs read identity's, and db/query-owners.yaml assigns each
// of them away from here. Measurement is allowed to read anything; it owns only
// what it writes.
//
// The requirement IDs it carries are 02:O11Y-004 and BETA-003/004/005. ADR-032
// §1 lists PDM and NFR against Policy & Usage: those cover the retention rule
// this package obeys, not the events it writes. It is a separate Supporting
// context from `policy` and is deliberately not merged into it (ADR-032 §1,
// DDD-014) — the only thing the two share is the word "retention".
//
// # Relationships (ADR-032 §2)
//
//	catalog -> analytics    synchronous query (Customer–Supplier) with the
//	                        supplier's answer thrown away: catalog calls
//	                        SearchPerformed and SkillDetailViewed fire-and-forget.
//	                        A measurement may never fail a search (ADR-029 決策 1).
//	analytics -> identity   synchronous, iron rule 3's entry point: the workspace
//	                        on an event comes from the session, never from the
//	                        request.
//	identity -> analytics   inverted at the composition root. PurgeWorkspace is
//	                        this context's share of an account deletion, and it
//	                        arrives in identity as an injected WorkspacePurge
//	                        rather than an import, because every context imports
//	                        identity and so identity may import none (ADR-034).
//	packaging               no edge in either direction. download_started is
//	                        emitted by Handler.DownloadStartedOn, which apiserver
//	                        wraps around packaging's DownloadContent handler, so
//	                        the download path carries no analytics import.
//
// No domain events are published or consumed here, and that is a rule rather
// than a gap: nothing in the product may react to a measurement, because a
// measurement is allowed to be wrong.
//
// # Public face
//
// Service.Sessions (the middleware, wrapped around the whole mux — the funnel's
// first segment happens on the public catalogue where no session middleware
// runs), SessionID, ArrivalFromRequest, the three recording methods, Enabled,
// PurgeExpired, PurgeWorkspace, and Handler.DownloadStartedOn.
//
// There is deliberately no read API. GET /policy/data-retention discloses what
// would be collected and for how long; it does not serve what was collected.
// A context that wants a product answer asks the domain table that owns it.
//
// # What is deliberately not here
//
// Not the funnel segments a domain table answers — those readings stay where the
// fact lives, and duplicating one here would create a second number that drifts.
// Not the evaluation feedback channel (PUT /runs/{id}/evaluation/feedback, owned
// by eval): merging "was this judgement useful" with "what did you want that you
// could not have" produces one bucket that answers neither. Not the session
// credential, which is identity's. Not the retention rules of other data classes:
// the download artifact's is policy's, trace's is a partition drop, and this
// package's own knob is separate from all of them. `analytics_events` is
// partitioned by month like `trace_events` and is rolled by the same generic
// job (MaintainPartitions, `maintenance rotate-partitions`), but the mechanism
// being shared is not the policy being shared: the window is still this
// package's ANALYTICS_RETENTION and trace's is still TRACE_RETENTION.
//
// # Failure modes
//
// Retention zero is the shipped default and means silence: no cookie on the
// visitor's browser, no row, Enabled false everywhere. Recording is best effort
// in the other direction too — a dropped event is a hole in a measurement, while
// a search that failed because a measurement failed is a broken product.
//
// PurgeWorkspace de-identifies rather than deletes (ADR-029 決策 5): the funnel
// is compared quarter over quarter and removing a departed user's events would
// rewrite last quarter's numbers. If it is not injected, identity refuses the
// entire account purge rather than committing one that silently skipped a
// context.
package analytics
