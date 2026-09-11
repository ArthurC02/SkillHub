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

type Plan struct {
	CreatingPolls int
	RunningPolls  int

	StuckRunning bool

	FinalState   run.ProviderRunState
	ResultStatus string
	ErrorClass   string

	OmitResult bool
}

type Fake struct {
	*httptest.Server
	Name  string
	Token string

	instance string

	mu         sync.Mutex
	runs       map[string]*fakeRun
	byKey      map[string]string
	bodies     map[string]string
	seq        int
	dispatches int
	destroys   int

	Capability *run.ProviderCapability

	DispatchStatuses []int

	OnDispatch func(runID string, attempt int)

	Plan Plan

	DestroyStatus int
}

type fakeRun struct {
	runID, attemptID, providerRunID string
	attempt                         int
	polls                           int
	cancelled                       bool
	createdAt                       time.Time
}

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

func randomID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (f *Fake) Provider() *run.Provider { return run.NewProvider(f.Name, f.URL, f.Token) }

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
	c.Isolation.Level = "container"
	c.Isolation.Rootless = true
	c.Isolation.DedicatedWorkspacePerRun = true

	c.MaxResourcesUnenforced = nil
	c.Isolation.ReapsDetachedDescendants = &reaps
	c.Network.EgressModes = []string{"default_deny"}

	c.Network.EgressUnenforced = false
	c.Availability.ConcurrentRunSlots = 4
	c.Availability.Healthy = &healthy
	return c
}

func (f *Fake) Dispatches() int { f.mu.Lock(); defer f.mu.Unlock(); return f.dispatches }

func (f *Fake) Destroys() int { f.mu.Lock(); defer f.mu.Unlock(); return f.destroys }

func (f *Fake) Live() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.runs) }

func (f *Fake) Seed(runID, attemptID string, createdAt time.Time) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.create(runID, attemptID, 1, createdAt).providerRunID
}

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
		if f.OnDispatch != nil {
			f.OnDispatch(req.RunID, req.Attempt)
		}
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

// create assumes the caller already holds f.mu.
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

		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only active=true is served"})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	list := run.ProviderRunList{Provider: f.Name, Runs: []run.ProviderRun{}, ObservedAt: time.Now()}
	for _, fr := range f.runs {
		view := f.view(fr)
		view.Result = nil
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

	fr.cancelled = true
	writeJSON(w, http.StatusAccepted, f.view(fr))
}

func (f *Fake) destroyRun(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroys++

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

	w.WriteHeader(http.StatusNoContent)
}

// view assumes the caller already holds f.mu.
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
