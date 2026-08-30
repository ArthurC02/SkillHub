// Package run is the current Go package for ADR-038's proposed product domain
// 「Skill 試跑執行」: it lets creators run, cancel or recover one controlled Skill
// trial. Its stable Boundary ID/package slug remains `run`; ADR-038 is Proposed
// and does not rename this package, its owner facts or its path.
// It owns the Run lifecycle: the standard state machine (RUN-002/004), the platform-to-provider identity mapping (RUN-003), and the queue entry point.
//
// One package, four file groups. They are groups rather than sub-packages on
// purpose: the boundary is real but the coupling is a Service method call, and
// splitting it would buy export churn and an import cycle rather than isolation.
//
//	aggregate    statemachine.go — the transition table, terminality, which
//	             successor is the happy one (NextOnSuccess/HappyPath, so no caller
//	             has to know the order a row is written in), and the one write path
//	             (Transition/record) that applies them all. Nothing else may write
//	             runs.status. Pure rules at the top, testable without a DB.
//
//	application  service.go (Service, Create, reads), schedule.go, job.go, halt.go,
//	             cleanup.go, supervisor.go, preflight.go, quota.go, gateb.go,
//	             grants.go — orchestration and policy. These decide *whether* and
//	             *when* a run moves; the aggregate decides whether the move is legal.
//
//	gateway      provider.go, gateway.go — the anti-corruption layer over the
//	             sandbox provider contract and the model gateway. Their wire types
//	             stop at those files.
//
//	http         http.go (plus the Handler methods that live beside their feature
//	             in halt.go and preflight.go) — transport only.
//
// Iron rule 5: this Postgres state machine is the single source of truth for run
// state. Nothing else — not a provider, not any in-process state on the Python
// side — may decide where a run is.
//
// ADR-016 rule 1 words that second example as "a LangGraph checkpoint", and this
// comment quoted it until 2026-08-30. LangGraph was never adopted (ADR-016's
// dated correction; uv.lock carries no such row), and a Go reader has no way to
// know that — so the sentence read as if those checkpoints exist here and are
// merely outranked. The rule does not depend on the example: Python owns no
// cross-request state at all today, which satisfies it outright rather than
// narrowly.
//
// The data contract this domain hands to a provider is
// contracts/openapi/sandbox-provider.yaml (iron rule 12). No provider exists yet;
// see job.go for what happens meanwhile.
package run
