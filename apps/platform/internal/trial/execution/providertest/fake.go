// Package providertest is an in-repo sandbox provider that implements
// contracts/openapi/sandbox-provider.yaml, for the contract test suite (RUN-009)
// and for the run-orchestration integration tests.
//
// It is a fake, not a mock: it holds real state, and every semantic the contract
// promises — idempotent dispatch on (run_id, attempt), 409 on a re-send with
// different content, 202 from cancel even on a terminal run, a DELETE with no 404,
// active-only listings with an observed_at — is implemented here rather than
// asserted about. That is what lets one suite run against this and against a real
// provider without changing a line (see SKILLHUB_PROVIDER_CONTRACT_URL).
//
// It runs no code and isolates nothing. It exists to exercise the *protocol*, and
// nothing here says anything about whether a real provider is safe (ADR-015,
// SEC-009).
package providertest

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// Plan is how a dispatched run behaves as it is polled. Zero value: one poll
// answers `running`, the next completes successfully.
type Plan struct {
	// CreatingPolls and RunningPolls are how many reads answer each state before
	// the run moves on.
	CreatingPolls int
	RunningPolls  int
	// StuckRunning keeps the run in `running` forever, for wall-clock tests.
	StuckRunning bool
	// FinalState and ResultStatus are the terminal answer. Zero values are
	// `completed` and `succeeded`.
	FinalState   run.ProviderRunState
	ResultStatus string
	ErrorClass   string
	// OmitResult returns a terminal state with no result, which the contract
	// forbids — the platform has to survive a provider that does it anyway.
	OmitResult bool
}

// Fake is a running provider. Close it with Close (httptest.Server).
type Fake struct {
	*httptest.Server
	Name  string
	Token string

	// instance keeps this fake's handles unique across fakes; see randomID.
	instance string

	mu         sync.Mutex
	runs       map[string]*fakeRun // provider_run_id -> run
	byKey      map[string]string   // "run_id/attempt" -> provider_run_id
	bodies     map[string]string   // "run_id/attempt" -> canonical request
	seq        int
	dispatches int
	destroys   int

	// Capability overrides what GET /capability answers. Nil means
	// DefaultCapability(Name).
	Capability *run.ProviderCapability
	// DispatchStatuses are answered by successive POST /runs calls before any run
	// is created — one entry per call, and the list is consumed. Used to drive the
	// bounded-retry path (503, 429) and the refusal path (422).
	DispatchStatuses []int
	// Plan governs every run this provider creates.
	Plan Plan
	// DestroyStatus, when >= 400, makes DELETE fail with that status and keep the
	// sandbox. A provider that cannot tear one down is the only way to observe a
	// leak that survives a scan round (SBX-012).
	DestroyStatus int
}

type fakeRun struct {
	runID, attemptID, providerRunID string
	attempt                         int
	polls                           int
	cancelled                       bool
	createdAt                       time.Time
}

// New starts a fake provider. token may be empty, in which case no Authorization
// header is required — a real provider always requires one, so the contract suite
// passes a token.
func New(name, token string) *Fake {
	f := &Fake{
		Name:     name,
		Token:    token,
		instance: randomID(),
		runs:     map[string]*fakeRun{},
		byKey:    map[string]string{},
		bodies:   map[string]string{},
	}
	f.Server = httptest.NewServer(f.routes())
	return f
}

// randomID keeps one fake's handles out of another's id space. run_attempts has a
// unique index on (provider, provider_run_id), so two fakes of the same name that
// both started counting at 1 would collide on the *second* test to run — which is
// exactly what a real provider does when it recycles handles across a restart, and
// exactly why handles are opaque.
func randomID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Provider returns a client pointed at this fake.
func (f *Fake) Provider() *run.Provider { return run.NewProvider(f.Name, f.URL, f.Token) }

// DefaultCapability is a provider that can run everything the platform asks for
// by default. Tests that need a mismatch narrow a copy of it.
func DefaultCapability(name string) run.ProviderCapability {
	healthy, reaps := true, true
	c := run.ProviderCapability{
		Provider: name,
		Runtimes: []run.RuntimeSupport{{
			Runtime:          "claude_agent_sdk",
			Versions:         []string{"0.1.0", "0.2.0"},
			AgentIntegration: []string{"in_sandbox_sdk"},
		}},
		MaxResources: run.DefaultResourceLimits(),
	}
	c.Isolation.Level = "container" // honest name for a fake on a developer machine
	c.Isolation.Rootless = true
	c.Isolation.DedicatedWorkspacePerRun = true
	// Declared explicitly, both of them, because the default this fake stands for
	// is a provider that answers honestly: every ceiling it names it enforces, and
	// it does end a descendant that left its process group. A test that wants the
	// dishonest shape narrows a copy - which is the point of these two fields
	// being here at all, since the platform side had no consumer for either until
	// Match() and ProviderSummary grew one.
	c.MaxResourcesUnenforced = nil
	c.Isolation.ReapsDetachedDescendants = &reaps
	c.Network.EgressModes = []string{"default_deny"}
	// The third field in that same family, and explicit for the same reason:
	// this fake stands for a node whose declaration is a boundary. A test that
	// wants a node declaring an egress mode it does not enforce sets it on a
	// copy.
	c.Network.EgressUnenforced = false
	c.Availability.ConcurrentRunSlots = 4
	c.Availability.Healthy = &healthy
	return c
}

// --- counters, for assertions -------------------------------------------------

// Dispatches is how many POST /runs calls created a new run. A re-send of the same
// (run_id, attempt) does not count: that is the point of the idempotency key.
func (f *Fake) Dispatches() int { f.mu.Lock(); defer f.mu.Unlock(); return f.dispatches }

// Destroys counts every DELETE, including the repeats that must stay safe.
func (f *Fake) Destroys() int { f.mu.Lock(); defer f.mu.Unlock(); return f.destroys }

// Live is how many sandboxes are still held.
func (f *Fake) Live() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.runs) }

// Seed plants a sandbox the platform never dispatched, with a chosen age. It is
// how an orphan is created without a race.
func (f *Fake) Seed(runID, attemptID string, createdAt time.Time) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.create(runID, attemptID, 1, createdAt).providerRunID
}

// --- routing -----------------------------------------------------------------

func (f *Fake) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /capability", f.auth(f.capability))
	mux.HandleFunc("POST /runs", f.auth(f.createRun))
	mux.HandleFunc("GET /runs", f.auth(f.listRuns))
	mux.HandleFunc("GET /runs/{id}", f.auth(f.getRun))
	mux.HandleFunc("DELETE /runs/{id}", f.auth(f.destroyRun))
	mux.HandleFunc("POST /runs/{id}/cancel", f.auth(f.cancelRun))
	return mux
}

func (f *Fake) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if f.Token != "" && r.Header.Get("Authorization") != "Bearer "+f.Token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid provider token"})
			return
		}
		next(w, r)
	}
}

func (f *Fake) capability(w http.ResponseWriter, _ *http.Request) {
	c := DefaultCapability(f.Name)
	if f.Capability != nil {
		c = *f.Capability
	}
	writeJSON(w, http.StatusOK, c)
}

func (f *Fake) createRun(w http.ResponseWriter, r *http.Request) {
	var req run.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed body"})
		return
	}
	if req.RunID == "" || req.Attempt < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id and attempt are required"})
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	key := fmt.Sprintf("%s/%d", req.RunID, req.Attempt)
	canonical, _ := json.Marshal(req)

	// Idempotency comes first: a re-send of a pair that already exists is answered
	// from state, never from the injected failure list. Otherwise a provider having
	// a bad minute could lose a sandbox it already created.
	if existing, ok := f.byKey[key]; ok {
		if f.bodies[key] != string(canonical) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "this (run_id, attempt) was dispatched with different content",
			})
			return
		}
		writeJSON(w, http.StatusOK, f.view(f.runs[existing]))
		return
	}

	if len(f.DispatchStatuses) > 0 {
		status := f.DispatchStatuses[0]
		f.DispatchStatuses = f.DispatchStatuses[1:]
		if status >= 400 {
			if status == http.StatusUnprocessableEntity {
				writeJSON(w, status, run.RunError{
					Class: "capability_mismatch", Message: "this provider cannot enforce the requested limits",
				})
				return
			}
			writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
			return
		}
	}

	created := f.create(req.RunID, req.RunAttemptID, req.Attempt, time.Now())
	f.byKey[key] = created.providerRunID
	f.bodies[key] = string(canonical)
	f.dispatches++
	writeJSON(w, http.StatusCreated, f.view(created))
}

// create allocates a sandbox. Caller holds the lock.
func (f *Fake) create(runID, attemptID string, attempt int, createdAt time.Time) *fakeRun {
	f.seq++
	fr := &fakeRun{
		runID: runID, attemptID: attemptID, attempt: attempt,
		providerRunID: fmt.Sprintf("%s-%s-%d", f.Name, f.instance, f.seq),
		createdAt:     createdAt,
	}
	f.runs[fr.providerRunID] = fr
	return fr
}

func (f *Fake) getRun(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fr, ok := f.runs[r.PathValue("id")]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no run with this handle"})
		return
	}
	fr.polls++
	writeJSON(w, http.StatusOK, f.view(fr))
}

func (f *Fake) listRuns(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("active") == "false" {
		// Not an empty list: an empty list would read as "no finished runs" from an
		// endpoint that never had any to give.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only active=true is served"})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	list := run.ProviderRunList{Provider: f.Name, Runs: []run.ProviderRun{}, ObservedAt: time.Now()}
	for _, fr := range f.runs {
		view := f.view(fr)
		view.Result = nil // listings omit the result by contract
		list.Runs = append(list.Runs, view)
	}
	writeJSON(w, http.StatusOK, list)
}

func (f *Fake) cancelRun(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fr, ok := f.runs[r.PathValue("id")]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no run with this handle"})
		return
	}
	// 202 even when the run is already terminal: the caller polls on an interval
	// and the run can finish between the read and the cancel.
	fr.cancelled = true
	writeJSON(w, http.StatusAccepted, f.view(fr))
}

func (f *Fake) destroyRun(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroys++
	// DestroyStatus lets a test hold a sandbox that will not die - the leak the
	// reconciler's consecutive-round threshold exists for (ADR-022 X-03, SBX-012).
	// The sandbox stays in f.runs, so the next scan sees the same handle again.
	if f.DestroyStatus >= 400 {
		w.WriteHeader(f.DestroyStatus)
		return
	}
	id := r.PathValue("id")
	if fr, ok := f.runs[id]; ok {
		delete(f.runs, id)
		delete(f.byKey, fmt.Sprintf("%s/%d", fr.runID, fr.attempt))
		delete(f.bodies, fmt.Sprintf("%s/%d", fr.runID, fr.attempt))
	}
	// No 404: a handle nothing is held for is already in the state the caller asked
	// for. This is what makes a crashed cleanup safe to repeat.
	w.WriteHeader(http.StatusNoContent)
}

// view renders one run at its current point in the plan. Caller holds the lock.
func (f *Fake) view(fr *fakeRun) run.ProviderRun {
	state, resultStatus := f.stateOf(fr)
	created := fr.createdAt
	view := run.ProviderRun{
		RunID: fr.runID, RunAttemptID: fr.attemptID,
		Provider: f.Name, ProviderRunID: fr.providerRunID,
		State: state, CreatedAt: &created, ObservedAt: time.Now(),
	}
	if fr.cancelled {
		at := created
		view.CancelRequestedAt = &at
	}
	if state.Terminal() && !f.Plan.OmitResult {
		result := run.RunResult{RunID: fr.runID, RunAttemptID: fr.attemptID, Status: resultStatus}
		if resultStatus != "succeeded" {
			class := f.Plan.ErrorClass
			if class == "" {
				class = "execution"
			}
			result.Error = &run.RunError{Class: class, Message: "fake provider: " + resultStatus}
		}
		view.Result = &result
	}
	return view
}

func (f *Fake) stateOf(fr *fakeRun) (run.ProviderRunState, string) {
	if fr.cancelled {
		return run.ProviderStateCancelled, "cancelled"
	}
	switch {
	case fr.polls <= f.Plan.CreatingPolls:
		return run.ProviderStateCreating, ""
	case f.Plan.StuckRunning, fr.polls <= f.Plan.CreatingPolls+f.Plan.RunningPolls:
		return run.ProviderStateRunning, ""
	}
	state := f.Plan.FinalState
	if state == "" {
		state = run.ProviderStateCompleted
	}
	status := f.Plan.ResultStatus
	if status == "" {
		status = "succeeded"
	}
	return state, status
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
