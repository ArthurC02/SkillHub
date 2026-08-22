// Package policy is the current Go package for ADR-038's proposed product domain
// 「創作者使用權益與資料生命週期」: it lets creators use the platform under
// understandable allowance and retention rules. Its stable Boundary ID/package
// slug remains `policy`; ADR-038 is Proposed and does not rename it.
// It is the Policy & Usage context (ADR-032 §1): what a workspace is allowed to consume, and how long what it produces is kept.
//
// Two rules live here today, and they arrived from two different places:
//
//	quota      the PDM-010 free run allowance and its counting rule, previously
//	           inside internal/run (ADR-028 決策 2)
//	retention  whether a Download Artifact may be created at all, previously a
//	           bare `<= 0` check inside internal/packaging (PDM-006,
//	           debt ledger GOV-RETENTION-001)
//
// **This package decides; it does not act.** EnforceQuota answers "may this
// workspace start another run" and hands back a refusal reason — the caller runs
// it, and run calls it from inside the create-run transaction, under the advisory
// lock requireRunSlot already holds. That location is the decision of ADR-028
// 決策 2 and moving the rule here did not move the enforcement point: it is still
// the one moment where a run is about to exist and nothing has been spent yet.
// DownloadRetention.Period is the same shape — packaging asks, and refuses its own
// request when the answer is "not configured".
//
// Two boundaries worth stating, because both were live questions:
//
//   - `analytics` stays a separate Supporting context and is **not** merged in
//     here (DDD-014). Funnel measurement is ADR-029's own concern with its own
//     retention knob, its own disclosure endpoint and its own audit boundary; the
//     only thing it shares with this package is the word "retention".
//   - The workspace concurrency ceiling stays in `run`
//     (run.MaxConcurrentRunsPerWorkspace). It is a Run Orchestration invariant —
//     how many runs may be in flight — not an allowance of anything; run reports
//     it alongside the allowance because a user wants all four limits in one
//     place, and that is a display decision, not ownership.
//
// This is where billing goes when it arrives (ADR-011's Policy & Usage ownership
// list). It imports no bounded context, and nothing in it may grow one:
// the depguard `policy` rule in apps/platform/.golangci.yml is that in machine
// form.
package policy
