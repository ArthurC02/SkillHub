// Package trace is the Run Trace context (ADR-032 §1 Supporting, requirement
// prefix TRACE; ADR-009's Run Trace plane): the masking, storage and reading of
// what a run actually did.
//
// # What it owns
//
// One table, `trace_events` (db/query-owners.yaml: trace.sql, plus
// FindLiveTraceEvents, which sits in evaluation.sql because of who quotes it,
// not because of who owns it). It is declared immutable and is the
// execution record itself — TRACE-001 calls it 執行事實, and retention on it is a
// partition drop, never an UPDATE or a DELETE.
//
// It owns two things that are not tables and matter more. The first is the
// ingestion credential (token.go): a signed, self-verifying token in the URL that
// authorizes exactly "append events to this one (run_id, attempt)" and can neither
// read anything nor name a workspace. The second is the masker (mask.go), and the
// order it runs in is the whole security argument of this package — verify the
// token, resolve the scope from the platform's own data, validate the envelope,
// mask, then store. Nothing reaches the database before the masker has run.
//
// # Relationships (ADR-032 §2)
//
//	run -> trace       synchronous write. [RecordOrchestratorEvent] takes the
//	                   caller's transaction so a control-plane event commits with
//	                   the state change it describes (iron rule 9).
//	eval -> trace      synchronous read. The judge's evidence and the
//	                   deterministic checks are stored events, read through
//	                   Service and the exported event-type vocabulary, including
//	                   [Service.LiveEvents] for whether a citation still resolves.
//	                   The Service is injected from the composition root — DDD-004
//	                   removed two methods that built one on the spot, which also
//	                   built one with a nil Signer.
//	trace -> identity  synchronous, iron rule 3. The read routes are session
//	                   scoped and answer "no such run, or not yours" as one error,
//	                   because existence is private (WS-006).
//
// The sandbox posting to POST /internal/trace/{token} is not a context relation
// at all: it is the ADR-001 trust boundary. The execution plane never touches the
// database — it posts JSON and the control plane decides everything (iron rule 2).
// That endpoint takes no session and no deployment-wide provider credential,
// because a credential covering every run must not be able to append to one.
//
// Nothing here publishes or consumes a domain event.
//
// # Public face
//
// [RecordOrchestratorEvent] is the only write another context may perform, and
// [MaskingActivity] is the one read that is not about a single run: it is what
// the masker did across a window, which internal/run's supervisor reads as
// 02:SEC-010's TraceMaskingStopped criterion. Both take the caller's *gen.Queries.
// DDD-033 added the latter, and [Service.LiveEvents], so that eval and run could
// stop calling this package's queries directly (ADR-035 G 組).
//
// [Service] carries the read shapes eval and the UI need — [Summary],
// [AdvancedView], [ErrorSummary], [StreamHealth] — and the event-type and source
// constants are exported so a reader classifies events by the same names the
// writer used rather than by string literals of its own. [Signer], [Handler] and
// [IngestPath] are for the composition root and the router.
//
// [Placeholder] and [Masker] are exported for the same reason the constants are:
// a test or a reader that needs to know what a redaction looks like should read
// the value this package writes, not a copy.
//
// # What is deliberately not here
//
// Not run state. A trace event describes what happened; where a run *is* is the
// Postgres state machine in internal/run and nothing else (iron rule 5). An error
// event is not a failed run.
//
// Not the verdict. `runs.status` and `evaluations.overall` answer different
// questions (ADR-025) and this package answers neither — it supplies the
// evidence eval quotes and re-verifies.
//
// Not the schema. contracts/events/trace-event.schema.json is the contract and
// event.go is hand-written against it, the way run/provider.go is against the
// sandbox contract; tools/contracts/validate_trace_events.py plus this package's
// golden sample are what keep the two honest, since no codegen is wired for
// contracts/ yet.
//
// # Failure modes
//
// Masking is on the way in, not on the way out, because once a plaintext key is
// in a jsonb column it has already leaked, backups included. 0019's CHECK
// (masked) is the second line: no code path can store an unmasked row. Known
// values (the ingestion token above all) are matched exactly; patterns catch
// credential shapes the platform did not issue and are necessarily incomplete,
// which is why the first category exists. Replacement is the literal
// [Placeholder] — a length-preserving or partial mask leaks entropy about the
// secret.
//
// Duplicates are counted, not treated as errors: delivery is at-least-once and a
// producer retrying after an uncertain response is doing the right thing.
//
// Retention is a partition drop, and as of DDD-032 there is a job that performs
// it: [MaintainPartitions], run by `maintenance rotate-partitions`. Three things
// about it are worth knowing before relying on it. It only runs when the
// deployment sets TRACE_RETENTION — unset means no month is ever created or
// dropped, deliberately, because PDM-006 has not ratified a number and this job
// deletes. It is invoked by the deployment's cron and not by anything in this
// process (iron rule 6). And it does not reach the rows already sitting in
// `trace_events_default`, the catch-all 0019 added: every event written between
// 2026-09 and the first run of this job is there, and getting them into a month
// is a one-off drain the job's error message spells out when it hits one.
//
// The consequence for readers is unchanged: evidence stored elsewhere must
// re-answer availability at read time rather than trusting a stored flag, which
// is exactly what eval does (ADR-026 decision 2). A dropped month takes its
// events with it and no reference to them is repaired.
package trace
