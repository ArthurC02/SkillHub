package envx

import (
	"context"
	"sort"
	"strings"
	"time"
)

// The deployment capability table (05 R-36 第二段): what this deployment can
// do, what each capability needs, and what a user meets when it is not there.
//
// # Why it lives in envx
//
// This package already owns the decision every deployment variable makes — the
// four idioms in the package comment above name what an UNSET value means. It
// never named the other half: what that variable BLOCKS. Both halves are
// properties of the same variable, and keeping them apart is what let
// DOWNLOAD_ARTIFACT_RETENTION exist in .env.example, be read by the code, and
// have no place at all that said it stops packaging (R-36's own example).
//
// # The state that did not exist, and why the whole table is here for it
//
// Before this, the launcher decided a capability was present with
// `!process.env[n]` — the variable is set, therefore the feature works. On
// 2026-09-01 that produced three green lights in a row over a service that
// could not perform a single one of its four jobs: the launcher ticked
// 跨語言意圖搜尋 because LLM_SERVICE_URL was set, the platform's /healthz
// answered ok because it is a liveness probe and correctly says nothing about
// dependencies, and apps/llm's own /healthz answered ok while an unset
// LLM_SERVICE_TOKEN made every one of its capability endpoints answer 503. The
// only honest signal in the chain was the search response's `degraded` flag,
// arriving last (04 丙-118).
//
// None of those three was wrong on its own. The defect was in the composition,
// and nothing owned the composition. So the rule this file exists to enforce is
// a shape, not a check:
//
//	Ready is reachable ONLY by measurement.
//
// A capability whose variables are all present and which nothing probed is
// Unmeasured — a distinct answer, rendered distinctly, never a tick. That is
// the state the old table could not express, and every green light it printed
// was really this one.
type Readiness string

const (
	// Ready: a probe ran and succeeded. Nothing else produces this value.
	Ready Readiness = "ready"
	// Unmeasured: every declared precondition is present and no probe exists or
	// none ran. Configuration is not function; this says so instead of guessing.
	Unmeasured Readiness = "unmeasured"
	// Unavailable: a declared precondition is missing. Missing names which.
	Unavailable Readiness = "unavailable"
	// Broken: a probe ran and failed. Detail carries its reason, already safe to
	// show — probes must not put a secret in an error (iron rule 11).
	Broken Readiness = "broken"
)

// Capability is one thing a person can try, and what stands between them and it.
//
// Name, Without and Fix are operator-facing and in the interface language: this
// table is printed at boot on a machine where, for the person standing in front
// of it, that print is the only diagnostic they will get.
type Capability struct {
	// ID is stable and machine-readable; the launcher and the readiness endpoint
	// both key on it, so it must not be renamed casually.
	ID string
	// Name is what to call this capability to a person.
	Name string
	// Needs are the deployment variables without which it cannot work at all.
	Needs []string
	// Without is what a user meets when a precondition is missing. Not "it is
	// off" — the screen they will actually land on.
	Without string
	// Fix is how to supply what is missing.
	Fix string
	// Probe measures the capability. Nil means nobody can measure this one yet,
	// which is a fact about the deployment and is reported as Unmeasured rather
	// than rounded up to Ready.
	//
	// A probe must be cheap, must not spend money, and must not mutate anything:
	// /readyz is unauthenticated and can be polled.
	Probe func(context.Context) error `json:"-"`
}

// Status is one row of the answer.
type Status struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Readiness   Readiness `json:"readiness"`
	Missing     []string  `json:"missing,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	Without     string    `json:"without,omitempty"`
	Fix         string    `json:"fix,omitempty"`
	MeasuredFor string    `json:"measured_for,omitempty"`
}

// Registry is the set of capabilities a build knows about.
type Registry struct {
	caps []Capability
}

// NewRegistry copies the table so a caller cannot mutate it after boot.
func NewRegistry(caps []Capability) *Registry {
	out := make([]Capability, len(caps))
	copy(out, caps)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return &Registry{caps: out}
}

// Capabilities returns the declared table, probes included.
func (r *Registry) Capabilities() []Capability { return r.caps }

// DeclaredVars is every variable the table mentions, deduplicated. The devctl
// checker compares this against .env.example so that adding a deployment
// variable without saying what it blocks fails CI (R-36).
func (r *Registry) DeclaredVars() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range r.caps {
		for _, n := range c.Needs {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Report answers "what can this deployment do right now".
//
// lookup reads a variable — injected rather than calling os.Getenv directly so
// a test can drive the whole table without touching process state.
//
// Probes run with the deadline the caller sets on ctx. A probe that returns an
// error makes the capability Broken and never Unavailable: those are different
// facts, and collapsing them is how "misconfigured" gets reported as "you did
// not turn this on".
func (r *Registry) Report(ctx context.Context, lookup func(string) string) []Status {
	out := make([]Status, 0, len(r.caps))
	for _, c := range r.caps {
		s := Status{ID: c.ID, Name: c.Name, Without: c.Without, Fix: c.Fix}
		for _, n := range c.Needs {
			if strings.TrimSpace(lookup(n)) == "" {
				s.Missing = append(s.Missing, n)
			}
		}
		switch {
		case len(s.Missing) > 0:
			s.Readiness = Unavailable
		case c.Probe == nil:
			// The important branch. Everything was configured and nobody looked.
			s.Readiness = Unmeasured
			s.Detail = "前提都在，但這個部署沒有辦法量它——「設定齊全」不等於「它會動」。"
		default:
			started := time.Now()
			if err := c.Probe(ctx); err != nil {
				s.Readiness = Broken
				s.Detail = err.Error()
			} else {
				s.Readiness = Ready
			}
			s.MeasuredFor = time.Since(started).Round(time.Millisecond).String()
		}
		out = append(out, s)
	}
	return out
}

// AllReady is true when every capability was measured and works. Unmeasured is
// deliberately not enough: a caller asking this question wants to know whether
// the deployment works, and "nobody looked" is not an answer to that.
func AllReady(rows []Status) bool {
	for _, s := range rows {
		if s.Readiness != Ready {
			return false
		}
	}
	return len(rows) > 0
}
