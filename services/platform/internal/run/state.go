// Package run owns the Run lifecycle: the standard state machine (RUN-002/004),
// the platform-to-provider identity mapping (RUN-003), and the queue entry point.
//
// Iron rule 5: this Postgres state machine is the single source of truth for run
// state. Nothing else — not a provider, not a LangGraph checkpoint — may decide
// where a run is.
//
// The data contract this domain hands to a provider is
// contracts/openapi/sandbox-provider.yaml (iron rule 12). No provider exists yet;
// see job.go for what happens meanwhile.
package run

import "github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"

// The standard lifecycle of ADR-004 / RUN-002:
//
//	queued → provisioning → preparing → running → evaluating
//	       → succeeded | failed | cancelled | timed_out
//
// `cleaning_up` is not in this machine. ADR-004 places cleanup *after* a terminal
// state so the execution outcome and the cleanup outcome are recorded separately;
// it lives in runs.cleanup_status (0004). A run that failed and whose sandbox was
// then successfully torn down is two different facts.
//
// successors is the whole legality rule, as data. Each non-terminal state may
// advance to exactly one next state, and may fail out to any of the three
// unhappy terminals from wherever it is:
//
//   - failed: at any point, from a policy rejection before dispatch to a broken
//     evaluation afterwards.
//   - cancelled: RUN-004 names all five non-terminal states as cancellable.
//   - timed_out: the wall clock (PDM-005 §5.2) covers queue wait as well as
//     execution, so a run can time out before a sandbox ever exists.
//
// succeeded is reachable only from evaluating: a run whose result was never
// judged has not succeeded, it has merely finished.
var successors = map[gen.RunStatus][]gen.RunStatus{
	gen.RunStatusQueued: {
		gen.RunStatusProvisioning,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusProvisioning: {
		gen.RunStatusPreparing,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusPreparing: {
		gen.RunStatusRunning,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusRunning: {
		gen.RunStatusEvaluating,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusEvaluating: {
		gen.RunStatusSucceeded,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	// Terminal: absent from the map, so every transition out of one is illegal.
	// The 0005 trigger enforces the same thing in the database.
}

// AllStatuses is every value of the run_status enum, in lifecycle order. Exported
// so the state machine test can assert over the full cross product rather than
// over a list someone remembered to update.
var AllStatuses = []gen.RunStatus{
	gen.RunStatusQueued,
	gen.RunStatusProvisioning,
	gen.RunStatusPreparing,
	gen.RunStatusRunning,
	gen.RunStatusEvaluating,
	gen.RunStatusSucceeded,
	gen.RunStatusFailed,
	gen.RunStatusCancelled,
	gen.RunStatusTimedOut,
}

// CanTransition reports whether a run may move from one status to another.
// Self-transitions are illegal: re-applying a state change is caught by the
// expected-from-status guard in TransitionRun, not silently accepted here.
func CanTransition(from, to gen.RunStatus) bool {
	for _, s := range successors[from] {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether a run in this status is finished executing. Cleanup
// may still be outstanding — that is runs.cleanup_status, not this.
func IsTerminal(s gen.RunStatus) bool {
	_, ongoing := successors[s]
	return !ongoing
}
