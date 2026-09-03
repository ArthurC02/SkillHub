package sandbox

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// p02Driver is the lifecycle half of the Driver interface, embedded so the rest
// panics: these tests drive dispatch and teardown and nothing else.
type p02Driver struct {
	Driver
	mu                sync.Mutex
	removed           []string
	done              chan struct{}
	startEntered      chan struct{}
	startBlock        chan struct{}
	startCanceled     chan struct{}
	startHook         func()
	ignoreStartCancel bool
	stopCalls         int
	removeCalled      chan string
	blockFirstRemove  bool
	firstRemove       sync.Once
	releaseRemove     chan struct{}
	hangStopAfter     int
	releaseStop       chan struct{}
	stopHadDeadline   bool
}

func newP02Driver() *p02Driver { return &p02Driver{done: make(chan struct{})} }

func (d *p02Driver) Start(ctx context.Context, _ string, _ RunRequest) error {
	if d.startEntered != nil {
		close(d.startEntered)
	}
	if d.startBlock != nil {
		if d.ignoreStartCancel {
			<-d.startBlock
		} else {
			select {
			case <-d.startBlock:
			case <-ctx.Done():
				if d.startCanceled != nil {
					close(d.startCanceled)
				}
				return ctx.Err()
			}
		}
	}
	if d.startHook != nil {
		d.startHook()
	}
	return nil
}
func (d *p02Driver) Wait(ctx context.Context, _ string) (Outcome, error) {
	select {
	case <-d.done:
	case <-ctx.Done():
	}
	return Outcome{}, nil
}
func (d *p02Driver) Stop(ctx context.Context, _ string, _ time.Duration) error {
	d.mu.Lock()
	d.stopCalls++
	call := d.stopCalls
	_, d.stopHadDeadline = ctx.Deadline()
	d.mu.Unlock()
	if d.hangStopAfter > 0 && call >= d.hangStopAfter {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.releaseStop:
		}
	}
	return nil
}
func (d *p02Driver) Remove(ctx context.Context, id string) error {
	block := false
	if d.blockFirstRemove {
		d.firstRemove.Do(func() { block = true })
	}
	d.mu.Lock()
	d.removed = append(d.removed, id)
	d.mu.Unlock()
	if d.removeCalled != nil {
		d.removeCalled <- id
	}
	if block {
		select {
		case <-d.releaseRemove:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (d *p02Driver) WorkloadDone(context.Context, string) (bool, error) { return false, nil }

// Rootless: Capability() asks the driver, and these tests read Capability() for
// its P-02 fields. Not part of the embedded panic, because a nil answer here
// would be a panic about something the test is not measuring.
func (d *p02Driver) Rootless() bool { return true }

func (d *p02Driver) ReleaseWorkload(context.Context, string) error { return nil }
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

type p02FlipHandler struct{ flip func() }

func (h *p02FlipHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *p02FlipHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "run dispatched" {
		h.flip()
	}
	return nil
}
func (h *p02FlipHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *p02FlipHandler) WithGroup(string) slog.Handler      { return h }

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
	probe.Check(context.Background(), &fakeProber{}, probeAt)

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

func TestP02BreachStartsEveryTeardownBeforeWaitingForOne(t *testing.T) {
	drv := newP02Driver()
	drv.blockFirstRemove = true
	drv.releaseRemove = make(chan struct{})
	drv.removeCalled = make(chan string, 2)
	m := p02Manager(drv)
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	probe.Check(context.Background(), &fakeProber{}, probeAt)
	m.p02 = probe

	first := p02Request()
	if _, _, err := m.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := p02Request()
	second.RunID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	second.RunAttemptID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if _, _, err := m.Create(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt)
	done := make(chan struct{})
	go func() {
		m.terminateEveryRun(probe.Result())
		close(done)
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-drv.removeCalled:
			seen[id] = true
		case <-time.After(time.Second):
			close(drv.releaseRemove)
			t.Fatalf("only %d of 2 teardowns started while another Remove was blocked", len(seen))
		}
	}
	close(drv.releaseRemove)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("breach teardown did not finish after the blocked driver was released")
	}
}

func TestCreateRefusesABreachBetweenRegistrationAndStart(t *testing.T) {
	drv := newP02Driver()
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	probe.Check(context.Background(), &fakeProber{}, probeAt)
	log := slog.New(&p02FlipHandler{flip: func() {
		probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt.Add(time.Second))
	}})
	// If Start is reached, make the later guard pass; this test then has teeth
	// specifically for the registration-to-Start window rather than borrowing
	// coverage from the post-Start guard.
	drv.startHook = func() {
		probe.Check(context.Background(), &fakeProber{}, probeAt.Add(2*time.Second))
	}
	m := NewManager(drv, p02Config(), log)
	m.p02 = probe

	if _, _, err := m.Create(context.Background(), p02Request()); err == nil {
		t.Fatal("dispatch crossed a P-02 breach that happened immediately after registration")
	}
	if len(m.List().Runs) != 0 {
		t.Fatal("registered run survived the P-02 refusal")
	}
	drv.mu.Lock()
	removed := len(drv.removed)
	drv.mu.Unlock()
	if removed != 1 {
		t.Fatalf("driver removals = %d, want the registered sandbox torn down once", removed)
	}
}

func TestCreateRefusesABreachThatOccursDuringDriverStart(t *testing.T) {
	drv := newP02Driver()
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	probe.Check(context.Background(), &fakeProber{}, probeAt)
	drv.startHook = func() {
		probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt.Add(time.Second))
	}
	m := p02Manager(drv)
	m.p02 = probe

	if _, _, err := m.Create(context.Background(), p02Request()); err == nil {
		t.Fatal("driver Start returned a sandbox after the P-02 probe failed")
	}
	if len(m.List().Runs) != 0 {
		t.Fatal("sandbox created during the breach remained registered")
	}
	drv.mu.Lock()
	removed := len(drv.removed)
	drv.mu.Unlock()
	if removed != 1 {
		t.Fatalf("driver removals = %d, want the started sandbox torn down once", removed)
	}
}

func TestAnUnknownP02ReadingRefusesNewWork(t *testing.T) {
	m := p02Manager(newP02Driver())
	m.p02 = &P02Probe{result: P02Result{State: P02Unknown, Detail: "dial tcp db.internal:5432: secret"}}

	_, _, err := m.Create(context.Background(), p02Request())
	if err == nil {
		t.Fatal("a node with no completed P-02 reading accepted a new run")
	}
	var re *RunError
	if !errors.As(err, &re) || !re.Retryable {
		t.Fatalf("refusal = %#v, want a retryable RunError", err)
	}
	if strings.Contains(re.Message, "db.internal") || strings.Contains(re.Message, "secret") {
		t.Fatalf("refusal exposed probe detail: %q", re.Message)
	}
}

func TestP02BreachCannotMissAStartingSandbox(t *testing.T) {
	drv := newP02Driver()
	drv.startEntered = make(chan struct{})
	drv.startBlock = make(chan struct{})
	drv.ignoreStartCancel = true
	drv.removeCalled = make(chan string, 2)
	m := p02Manager(drv)
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	probe.Check(context.Background(), &fakeProber{}, probeAt)
	m.p02 = probe

	created := make(chan error, 1)
	go func() {
		_, _, err := m.Create(context.Background(), p02Request())
		created <- err
	}()
	<-drv.startEntered
	probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt)
	terminated := make(chan struct{})
	go func() {
		m.terminateEveryRun(probe.Result())
		close(terminated)
	}()

	select {
	case <-drv.removeCalled:
	case <-time.After(time.Second):
		t.Fatal("breach did not attempt immediate teardown while Start was blocked")
	}
	select {
	case <-terminated:
	case <-time.After(time.Second):
		t.Fatal("breach teardown did not return while Start was blocked")
	}
	close(drv.startBlock)
	if err := <-created; err == nil {
		t.Fatal("Create reported success after its sandbox was revoked by a breach")
	}
	if len(m.List().Runs) != 0 {
		t.Fatal("sandbox that started during the breach remained managed and live")
	}
	drv.mu.Lock()
	removed := len(drv.removed)
	drv.mu.Unlock()
	if removed < 2 {
		t.Fatalf("removed %d times, want immediate teardown plus cleanup after Start returned", removed)
	}
}

func TestP02BreachCancelsAnInFlightStart(t *testing.T) {
	drv := newP02Driver()
	drv.startEntered = make(chan struct{})
	drv.startBlock = make(chan struct{})
	drv.startCanceled = make(chan struct{})
	m := p02Manager(drv)
	probe := NewP02Probe([]string{"db.internal:5432"}, 0, 0)
	probe.Check(context.Background(), &fakeProber{}, probeAt)
	m.p02 = probe

	type createResult struct {
		created bool
		err     error
	}
	done := make(chan createResult, 1)
	go func() {
		_, created, err := m.Create(context.Background(), p02Request())
		done <- createResult{created: created, err: err}
	}()
	<-drv.startEntered
	probe.Check(context.Background(), &fakeProber{reached: []string{"db.internal:5432"}}, probeAt)
	m.terminateEveryRun(probe.Result())

	select {
	case <-drv.startCanceled:
	case <-time.After(time.Second):
		t.Fatal("in-flight Start did not observe P-02 cancellation")
	}
	select {
	case result := <-done:
		if result.err == nil || result.created {
			t.Fatalf("Create after cancellation: created=%v err=%v, want refusal", result.created, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Create did not return after its Start was cancelled")
	}
	if len(m.List().Runs) != 0 {
		t.Fatal("cancelled Start remained in the manager")
	}
}

func TestCancelInterruptsAnInFlightStart(t *testing.T) {
	drv := newP02Driver()
	drv.startEntered = make(chan struct{})
	drv.startBlock = make(chan struct{})
	drv.startCanceled = make(chan struct{})
	m := p02Manager(drv)

	done := make(chan error, 1)
	go func() {
		_, _, err := m.Create(context.Background(), p02Request())
		done <- err
	}()
	<-drv.startEntered
	runs := m.List().Runs
	if len(runs) != 1 || runs[0].State != StateCreating {
		t.Fatalf("in-flight runs = %+v, want one creating run", runs)
	}
	if _, err := m.Cancel(context.Background(), runs[0].ProviderRunID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-drv.startCanceled:
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt the in-flight driver Start")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Create did not return its cancelled snapshot: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Create stayed blocked after cancellation")
	}
	final, err := m.Get(runs[0].ProviderRunID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != StateCancelled || final.Result == nil || final.Result.Status != ResultCancelled {
		t.Fatalf("cancelled in-flight Start became %+v", final)
	}
}

func TestCancelStopsAgainAfterAnUncooperativeStartReturns(t *testing.T) {
	drv := newP02Driver()
	drv.startEntered = make(chan struct{})
	drv.startBlock = make(chan struct{})
	drv.ignoreStartCancel = true
	m := p02Manager(drv)

	done := make(chan error, 1)
	go func() {
		_, _, err := m.Create(context.Background(), p02Request())
		done <- err
	}()
	<-drv.startEntered
	run := m.List().Runs[0]
	if _, err := m.Cancel(context.Background(), run.ProviderRunID); err != nil {
		t.Fatal(err)
	}
	close(drv.startBlock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	drv.mu.Lock()
	stops := drv.stopCalls
	drv.mu.Unlock()
	if stops != 2 {
		t.Fatalf("Stop calls = %d, want the cancel-time call and a post-Start call", stops)
	}
	close(drv.done)
	deadline := time.Now().Add(time.Second)
	for {
		final, err := m.Get(run.ProviderRunID)
		if err != nil {
			t.Fatal(err)
		}
		if final.State == StateCancelled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cancelled uncooperative Start stayed %+v", final)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPostStartCancelStopHasABoundedContext(t *testing.T) {
	drv := newP02Driver()
	drv.startEntered = make(chan struct{})
	drv.startBlock = make(chan struct{})
	drv.ignoreStartCancel = true
	drv.hangStopAfter = 2
	drv.releaseStop = make(chan struct{})
	t.Cleanup(func() { close(drv.releaseStop) })
	cfg := p02Config()
	cfg.CancelGrace = time.Millisecond
	m := NewManager(drv, cfg, slog.New(slog.DiscardHandler))

	done := make(chan error, 1)
	go func() {
		_, _, err := m.Create(context.Background(), p02Request())
		done <- err
	}()
	<-drv.startEntered
	run := m.List().Runs[0]
	if _, err := m.Cancel(context.Background(), run.ProviderRunID); err != nil {
		t.Fatal(err)
	}
	close(drv.startBlock)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("post-Start Stop ignored its bounded context")
	}
	drv.mu.Lock()
	hadDeadline := drv.stopHadDeadline
	drv.mu.Unlock()
	if !hadDeadline {
		t.Fatal("post-Start Stop received a context without a deadline")
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
