// Package eval is the control-plane half of the evaluation pipeline (EVAL-001):
// the Evaluation aggregate, the deterministic checks, the judge leg, and the
// improvement suggestions that hang off a verdict (EVAL-002).
//
// It answers a different question from internal/run. `runs.status` records what
// happened while a run executed; `evaluations.overall` records whether the task
// was achieved, and this package never writes the former (ADR-025). A run that
// finished `succeeded` with an evaluation of `not_met` is an ordinary consistent
// state, not a contradiction to be smoothed over.
//
// Everything here runs in the control plane and executes nothing (iron rule 1/2):
// trace events are read as stored JSON, artifacts as a manifest of names and
// hashes, packages as bytes handed to skillpkg.Validate. No user-supplied check
// script is run — that is untrusted content and would have to go back to a
// sandbox, which 02:EVAL-001's boundary clause puts outside the MVP.
//
// Five of EVAL-001's six problem classes are decided by rules over the platform's
// own records (deterministic.go). Only "task effect" reaches a model, through
// apps/llm (judge.go), and every reference that comes back is re-verified
// here before it is stored (ADR-026 defence 3).
//
// # The Evaluation aggregate (ADR-032 §4)
//
// One package, four file groups — groups rather than sub-packages for the same
// reason internal/run gives: the boundary is real, but splitting it would buy
// export churn rather than isolation.
//
//	aggregate    eval.go's begin/complete/fail — the three, and only three, write
//	             paths onto an `evaluations` row's verdict, plus the status and
//	             verdict vocabularies they enforce. Nothing else in the process
//	             may write that table's judgement columns.
//
//	application  judge.go, deterministic.go, suggest.go, apply.go, comparison.go,
//	             consumer.go, job.go, reconcile.go — they decide *what* a verdict
//	             says and *when* one is produced; the aggregate decides whether the
//	             write is legal at all.
//
//	http         http.go, suggestions_http.go — transport, plus SetFeedback, which
//	             is the one user-facing write and deliberately reaches only the
//	             current revision.
//
// # Invariants
//
// The aggregate root is one `evaluations` row: one judgement of one run, under
// one rubric, by one judge prompt. The invariants, in the order they bind:
//
//  1. A run has at most one *current* judgement. Enforced by the partial unique
//     index evaluations_current_key (run_id WHERE superseded_at IS NULL), not by
//     application discipline — two racing evaluators cannot both end up current.
//
//  2. A judgement, once `completed`, never changes. The 0024 trigger freezes every
//     column except feedback_helpful, feedback_comment, superseded_at and
//     updated_at. Everything a report will ever quote — overall, summary,
//     criterion_results, deterministic_findings, judge_model,
//     judge_prompt_version, rubric_version, evidence_complete, cost — is written
//     in one statement (complete) and is a fact from then on. This is what makes
//     "the verdict changed" mean "the yardstick changed" rather than "someone
//     edited it" (ADR-026 decision 1).
//
//  3. A revision settles once. complete and fail both carry `status = 'pending'`
//     in their predicate, so the ordinary worker and the recovery sweep can race
//     and exactly one of them owns the terminal; the loser sees no rows and
//     returns errEvaluationSettled or nil rather than overwriting a settled row.
//
//  4. superseded_at is stamped once and never cleared. The stamp is stamped by
//     the *next* evaluation and only onto the row that is still current
//     (`WHERE superseded_at IS NULL`), so a third evaluation cannot re-stamp the
//     first. Clearing it is not blocked by the trigger — the column has to stay
//     writable for invariant 1's mechanism — but the partial unique index refuses
//     it while another current row stands, so history cannot be un-superseded.
//
//  5. Superseding and inserting happen in one transaction, supersede first.
//     evaluations_current_key is not deferrable, so an insert that goes first
//     collides with the standing verdict instead of replacing it. begin also takes
//     an advisory transaction lock keyed on (workspace, run): the index would
//     still hold without it, but two callers would then race to a constraint
//     error rather than to a queue.
//
//  6. A pending revision is not superseded from under itself. begin refuses with
//     errEvaluationInProgress when the current row is still pending, so a
//     redelivered job joins the work in flight rather than orphaning it — and
//     does not pay the judge twice (ADR-026 decision 4: the judge is a metered
//     call).
//
//  7. Nothing here writes `runs`. A verdict that moved a run's terminal state
//     would rewrite a row 0005 froze, and would make the terminal state depend on
//     which rubric was in force (ADR-025).
//
//  8. `source` is set by this package, never by the model, and `overall` is
//     recomputed here from the stored criterion results rather than taken from
//     the judge's own proposal (EVAL-001 clause 5, ADR-026 defence 1/3).
//
//  9. Evidence is stored as reference *and* excerpt, and `available` is
//     re-answered at read time. trace_events is dropped by partition, so a stored
//     `available: true` would go on claiming the original is still there long
//     after retention removed it (ADR-026 decision 2).
//
// # What append-only does not constrain
//
// "Append-only" is about *judgements*, not about rows never being updated. There
// are four UPDATE statements against `evaluations`, and each one is legal for a
// different reason. The list is here because their existence is the easiest way
// to conclude, wrongly, that the rule has already been broken:
//
//   - SupersedeCurrentEvaluation — this *is* the append-only mechanism, not an
//     exception to it. It writes superseded_at and nothing else; the judgement it
//     marks is left exactly as it was read and cited. Its counterpart in the same
//     transaction is an INSERT, which is where the "append" happens.
//
//   - CompleteEvaluation — the first write of a verdict, not a rewrite of one.
//     The row is created *before* the judge is called so that an evaluation which
//     dies mid-flight is a visible `pending` state rather than a row that never
//     existed; the placeholders it carries until then (`undetermined`,
//     evidence_complete false) are what "no verdict yet" honestly looks like. The
//     `status = 'pending'` predicate is what keeps this the only such write.
//
//   - FailEvaluation — the other terminal for that same pending row: the judge
//     was unreachable or the evidence unreadable. It is not a lenient verdict and
//     not a deletion; the deterministic findings survive because they came from
//     the platform's own records and are still true when only the model leg broke.
//
//   - SetEvaluationFeedback — the user's opinion *about* a verdict, which is not
//     part of the verdict. EVAL-001 clause 4 never said the user answers once, so
//     this replaces rather than appends, and 0024 lists both columns as writable
//     for exactly that reason. SetFeedback resolves the target through
//     GetCurrentEvaluation, so feedback can never be attached to a superseded
//     revision.
//
// Two more things append-only does not say. It does not say an evaluation is
// idempotent: calling Evaluate twice deliberately produces a second revision,
// which is why the job that calls it is unique per run while live. And it does
// not say suggestions are immutable — `evaluation_suggestions` rows carry a
// decision the user may change, and they hang off the evaluation that produced
// them precisely so that a re-evaluation gets its own set instead of appending to
// somebody else's.
//
// The data contract for a stored report is contracts/openapi/public.yaml
// (Evaluation, CriterionResult, EvidenceRef); the judge call is
// contracts/openapi/llm-internal.yaml (iron rule 12).
package eval
