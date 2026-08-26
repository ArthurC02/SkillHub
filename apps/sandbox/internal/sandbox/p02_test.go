package sandbox

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// p02Driver is the lifecycle half of the Driver interface, embedded so the rest
// panics: these tests drive dispatch and teardown and nothing else.
type p02Driver struct {
	Driver
	mu      sync.Mutex
	removed []string
	done    chan struct{}
}

func newP02Driver() *p02Driver { return &p02Driver{done: make(chan struct{})} }

func (d *p02Driver) Start(context.Context, string, RunRequest) error { return nil }
func (d *p02Driver) Wait(ctx context.Context, _ string) (Outcome, error) {
	select {
	case <-d.done:
	case <-ctx.Done():
	}
	return Outcome{}, nil
}
func (d *p02Driver) Stop(context.Context, string, time.Duration) error { return nil }
func (d *p02Driver) Remove(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.removed = append(d.removed, id)
	return nil
}
func (d *p02Driver) WorkloadDone(context.Context, string) (bool, error) { return false, nil }
func (d *p02Driver) ReleaseWorkload(context.Context, string) error      { return nil }
func (d *p02Driver) ReadTrace(context.Context, string, int64) ([]byte, bool, error) {
	return nil, false, nil
}
func (d *p02Driver) ReadArtifacts(context.Context, string) ([]byte, error) { return nil, nil }
func (d *p02Driver) Healthy(context.Context) bool                          { return true }

func p02Config() Config {
	return Config{
		Provider:       "test",
		Runtimes:       []RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"1"}}},
		MaxResources:   DefaultLimits,
		IsolationLevel: "container",
		EgressModes:    []string{"none"},
		Slots:          4,
	}
}

func p02Request() RunRequest {
	return RunRequest{
		RunID:          "11111111-1111-1111-1111-111111111111",
		RunAttemptID:   "22222222-2222-2222-2222-222222222222",
		Attempt:        1,
		WorkspaceID:    "33333333-3333-3333-3333-333333333333",
		SkillVersion:   PackageRef{SkillVersionID: "44444444-4444-4444-4444-444444444444", ContentHash: "sha256:abc"},
		TestCase:       TestCaseSnapshotRef{SnapshotID: "55555555-5555-5555-5555-555555555555", ContentHash: "sha256:def", UserPrompt: "probe"},
		Runtime:        RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"},
		ResourceLimits: DefaultLimits,
		Egress:         EgressPolicy{Mode: "none"},
		Trace:          TracePolicy{Level: "standard"},
	}
}

func p02Manager(drv Driver) *Manager {
	return NewManager(drv, p02Config(), slog.New(slog.DiscardHandler))
}

type fakeProber struct {
	reached []string
	err     error
	calls   int
}

func (f *fakeProber) ProbeEgress(context.Context, []string) ([]string, error) {
	f.calls++
	return f.reached, f.err
}

var probeAt = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

// The four states, and the two that are easy to collapse into each other are the
// point. ADR-022 §3 counts unknown as fail for acceptance and this code does not
// merge them anyway: `fail` is evidence of a hole, `unknown` is the absence of
// evidence either way, and they earn different actions.
func TestP02ProbeDistinguishesAHoleFromTheAbsenceOfEvidence(t *testing.T) {
	targets := []string{"db.internal:5432", "api.internal:8080"}
	for _, tc := range []struct {
		name   string
		probe  *P02Probe
		prober EgressProber
		want   P02State
	}{
		{"nothing answered", NewP02Probe(targets, 0, 0), &fakeProber{}, P02Pass},
		{"something answered", NewP02Probe(targets, 0, 0),
			&fakeProber{reached: []string{"db.internal:5432"}}, P02Fail},
		{"the probe could not run", NewP02Probe(targets, 0, 0),
			&fakeProber{err: errors.New("docker is down")}, P02Unknown},
		{"this driver cannot dial from a sandbox", NewP02Probe(targets, 0, 0), nil, P02Unknown},
		{"nobody said what must not be reachable", NewP02Probe(nil, 0, 0), &fakeProber{}, P02NotConfigured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.probe.Check(context.Background(), tc.prober, probeAt)
			if got.State != tc.want {
				t.Fatalf("state = %q, want %q (detail %q)", got.State, tc.want, got.Detail)
			}
			if !got.CheckedAt.Equal(probeAt) {
				t.Errorf("checked_at = %v, want %v", got.CheckedAt, probeAt)
			}
		})
	}
}

// A probe that could not run must never report `pass`. This is the shape that
// makes a resident check decorative: the healthy answer and the answer nobody
// took are both "no destination was reached".
func TestAProbeThatCouldNotRunIsNeverAPass(t *testing.T) {
	p := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	if got := p.Check(context.Background(), &fakeProber{err: errors.New("boom")}, probeAt); got.State == P02Pass {
		t.Fatal("a probe that failed to run reported pass")
	}
	// And before any reading at all.
	fresh := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	if got := fresh.Result(); got.State != P02Unknown {
		t.Fatalf("a node that has taken no reading reports %q, want unknown: starting at pass means a node "+
			"whose probe died on boot answers pass for its whole life", got.State)
	}
}

// checked_at is not decoration. A resident probe that stopped keeps answering
// `pass` with a timestamp that stops moving, and from outside the node that is
// the only way to see it.
func TestEveryReadingCarriesWhenItWasTaken(t *testing.T) {
	p := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	first := p.Check(context.Background(), &fakeProber{}, probeAt)
	second := p.Check(context.Background(), &fakeProber{}, probeAt.Add(5*time.Minute))
	if !second.CheckedAt.After(first.CheckedAt) {
		t.Fatalf("the second reading is not newer: %v then %v", first.CheckedAt, second.CheckedAt)
	}
	if p.Result().CheckedAt != second.CheckedAt {
		t.Error("Result() does not return the latest reading")
	}
}

// The node's own first action. The platform learns about this by polling, and
// between two polls is exactly the window in which untrusted code is talking to
// the core database.
func TestABreachTerminatesEveryLiveRunAndRefusesNewOnes(t *testing.T) {
	drv := newP02Driver()
	m := p02Manager(drv)
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	m.p02 = probe

	req := p02Request()
	if _, _, err := m.Create(context.Background(), req); err != nil {
		t.Fatalf("dispatch before the breach: %v", err)
	}
	if got := len(m.List().Runs); got != 1 {
		t.Fatalf("the node holds %d runs before the breach, want 1", got)
	}

	probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt)
	m.terminateEveryRun(probe.Result())

	if got := len(m.List().Runs); got != 0 {
		t.Errorf("the node still holds %d runs after a P-02 breach", got)
	}
	// And it stops taking work, without waiting for the platform to notice.
	req.RunAttemptID = "44444444-4444-4444-4444-444444444444"
	req.Attempt = 2
	_, _, err := m.Create(context.Background(), req)
	if err == nil {
		t.Fatal("a node in P-02 breach accepted a new run")
	}
	var re *RunError
	if !errors.As(err, &re) {
		t.Fatalf("refusal is %T, want a RunError the platform can classify", err)
	}
}

// The capability response is the only channel this contract has, so what it says
// is the whole reporting mechanism.
func TestCapabilityReportsTheProbeAndTakesTheNodeOutOfRotation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		state       P02State
		wantHealthy bool
	}{
		{"a clean reading serves", P02Pass, true},
		{"a breach does not", P02Fail, false},
		{"a reading nobody could take does not", P02Unknown, false},
		{"a node nobody configured still serves", P02NotConfigured, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := p02Manager(newP02Driver())
			m.p02 = &P02Probe{result: P02Result{State: tc.state, CheckedAt: probeAt}}
			c := m.Capability(context.Background())
			if c.Security == nil || c.Security.P02Probe == nil {
				t.Fatal("the capability response does not carry the probe's reading; " +
					"there is no other channel from this node to the platform")
			}
			if c.Security.P02Probe.State != tc.state {
				t.Errorf("reported state %q, want %q", c.Security.P02Probe.State, tc.state)
			}
			if c.Availability.Healthy != tc.wantHealthy {
				t.Errorf("healthy = %v, want %v for state %q", c.Availability.Healthy, tc.wantHealthy, tc.state)
			}
		})
	}
}

// A manager built without a probe reports no security block at all rather than
// an empty one: "this build has no probe" and "this probe found nothing" must
// not look the same on the wire.
func TestAManagerWithNoProbeClaimsNothing(t *testing.T) {
	m := p02Manager(newP02Driver())
	if c := m.Capability(context.Background()); c.Security != nil {
		t.Fatalf("a manager with no probe reported a security block: %+v", c.Security)
	}
}
