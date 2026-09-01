package envx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The invariant the whole table exists for: Ready is reachable only by
// measurement.
//
// 04 丙-118 is what its absence looked like. The launcher decided a capability
// was present with `!process.env[n]`, so a service restarted without its
// credential — answering 503 on every capability endpoint — printed as a green
// tick. Three greens in a row sat over it and the only honest signal arrived
// last. "Configured" and "works" are different facts and this is the type that
// keeps them apart.
func TestConfiguredWithoutAProbeIsNeverReady(t *testing.T) {
	reg := NewRegistry([]Capability{{
		ID:    "no_probe",
		Name:  "沒有人量得到的能力",
		Needs: []string{"PRESENT"},
	}})
	rows := reg.Report(context.Background(), func(string) string { return "value" })

	if got := rows[0].Readiness; got != Unmeasured {
		t.Fatalf("readiness = %q, want %q — every precondition was present and nothing measured anything, "+
			"which is exactly the state that used to print as a tick", got, Unmeasured)
	}
	if rows[0].Detail == "" {
		t.Error("Unmeasured with no detail is a state nobody can act on")
	}
	// And it must not satisfy the whole-deployment question either. A caller
	// asking AllReady wants to know whether the deployment works; "nobody
	// looked" is not an answer to that.
	if AllReady(rows) {
		t.Error("AllReady accepted a capability nothing measured")
	}
}

func TestTheFourAnswersAreDistinguishable(t *testing.T) {
	boom := errors.New("閘道沒有服務這個模型")
	reg := NewRegistry([]Capability{
		{ID: "a_ready", Needs: []string{"SET"}, Probe: func(context.Context) error { return nil }},
		{ID: "b_broken", Needs: []string{"SET"}, Probe: func(context.Context) error { return boom }},
		{ID: "c_unavailable", Needs: []string{"SET", "MISSING"}, Probe: func(context.Context) error { return nil }},
		{ID: "d_unmeasured", Needs: []string{"SET"}},
	})
	lookup := func(k string) string {
		if k == "SET" {
			return "x"
		}
		return ""
	}
	rows := reg.Report(context.Background(), lookup)
	got := map[string]Status{}
	for _, r := range rows {
		got[r.ID] = r
	}

	if got["a_ready"].Readiness != Ready {
		t.Errorf("a probe that succeeded is %q", got["a_ready"].Readiness)
	}
	if got["b_broken"].Readiness != Broken || !strings.Contains(got["b_broken"].Detail, "模型") {
		t.Errorf("a probe that failed is %q with detail %q; the reason has to survive",
			got["b_broken"].Readiness, got["b_broken"].Detail)
	}
	// Unavailable and Broken are different facts. Collapsing them is how
	// "misconfigured" gets reported as "you did not turn this on", and the probe
	// must not even run when a precondition is absent — it would be dialling
	// with half a configuration.
	if got["c_unavailable"].Readiness != Unavailable {
		t.Errorf("a missing precondition is %q, want %q", got["c_unavailable"].Readiness, Unavailable)
	}
	if len(got["c_unavailable"].Missing) != 1 || got["c_unavailable"].Missing[0] != "MISSING" {
		t.Errorf("Missing = %v, want it to name the one absent variable", got["c_unavailable"].Missing)
	}
	if got["d_unmeasured"].Readiness != Unmeasured {
		t.Errorf("no probe is %q, want %q", got["d_unmeasured"].Readiness, Unmeasured)
	}
}

// A variable that is present but blank is absent. Shells and .env files produce
// `FOO=` constantly, and treating that as configured is how a capability
// reports Unmeasured (or worse, probes with an empty credential) instead of
// naming what is missing.
func TestABlankVariableCountsAsMissing(t *testing.T) {
	reg := NewRegistry([]Capability{{ID: "x", Needs: []string{"BLANK"}}})
	for _, value := range []string{"", "   ", "\t"} {
		rows := reg.Report(context.Background(), func(string) string { return value })
		if rows[0].Readiness != Unavailable {
			t.Errorf("%q was treated as configured", value)
		}
	}
}

// The checker in tools/devctl reads this to compare the table against
// .env.example, so a variable named twice must not be reported twice and the
// order must not depend on map iteration.
func TestDeclaredVarsIsDeduplicatedAndStable(t *testing.T) {
	reg := NewRegistry([]Capability{
		{ID: "b", Needs: []string{"TWO", "ONE"}},
		{ID: "a", Needs: []string{"ONE"}},
	})
	for i := 0; i < 20; i++ {
		if got := strings.Join(reg.DeclaredVars(), ","); got != "ONE,TWO" {
			t.Fatalf("DeclaredVars = %q", got)
		}
	}
}

func TestAnEmptyTableIsNotReady(t *testing.T) {
	// A build that declares nothing has measured nothing. Answering "ready" here
	// would be this whole file's defect reintroduced at its own boundary.
	if AllReady(nil) {
		t.Error("AllReady said an empty table was ready")
	}
}
