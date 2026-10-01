package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func modelServing(t *testing.T, enrich http.HandlerFunc) Model {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/enrich-skill", enrich)
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, llmclient.EmbedResponse{Embeddings: [][]float32{make([]float32, 1536)}, Model: "e", Dimensions: 1536})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
}

func enriched(w http.ResponseWriter) {
	writeTestJSON(w, llmclient.EnrichSkillResponse{Summary: testEnrichedSummary, Model: "m", PromptVersion: "p"})
}

func skillsToEnrich(n int) []plannedSkill {
	admitted := make([]plannedSkill, n)
	for i := range admitted {
		admitted[i] = plannedSkill{pkg: testPackage()}
	}
	return admitted
}

func raiseTo(most *atomic.Int32, n int32) {
	for {
		prev := most.Load()
		if n <= prev || most.CompareAndSwap(prev, n) {
			return
		}
	}
}

func TestAnImportStopsEnrichingWhenItsWindowClosesAndLeavesTheRestForTheBackfill(t *testing.T) {
	var enrichCalls atomic.Int32
	release := make(chan struct{})
	s := &Service{LLM: modelServing(t, func(w http.ResponseWriter, r *http.Request) {
		if enrichCalls.Add(1) == 1 {
			enriched(w)
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})}
	t.Cleanup(func() { close(release) })
	admitted := skillsToEnrich(importEnrichmentConcurrency + 3)

	started := time.Now()
	got := s.enrichWithin(context.Background(), 300*time.Millisecond, admitted, pgtype.UUID{})

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("enrichment took %v; the window should have ended it", elapsed)
	}
	enrichedCount := 0
	for i, e := range got {
		if e.status == enrichmentEnriched {
			enrichedCount++
		} else if e.summary != testDescription {
			t.Errorf("skill %d left pending has summary %q, want its own description", i, e.summary)
		}
	}
	if enrichedCount != 1 {
		t.Errorf("%d skills enriched, want 1: only the call that answered may count", enrichedCount)
	}
	if n, most := enrichCalls.Load(), int32(importEnrichmentConcurrency+1); n != most {
		t.Errorf("the model was asked %d times, want %d: the first answer frees one slot, then every slot hangs until the window closes, and no call may start after it", n, most)
	}
}

func TestAnImportEnrichesSeveralSkillsAtOnceButNoMoreThanItsSlots(t *testing.T) {
	var inFlight, most atomic.Int32
	allSlotsBusy, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s := &Service{LLM: modelServing(t, func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		raiseTo(&most, n)
		if n == importEnrichmentConcurrency {
			once.Do(func() { close(allSlotsBusy) })
		}
		select {
		case <-allSlotsBusy:
			enriched(w)
		case <-r.Context().Done():
		case <-release:
		}
	})}
	t.Cleanup(func() { close(release) })
	admitted := skillsToEnrich(3 * importEnrichmentConcurrency)

	got := s.enrichWithin(context.Background(), 5*time.Second, admitted, pgtype.UUID{})

	for i, e := range got {
		if e.status != enrichmentEnriched {
			t.Errorf("skill %d status = %q, want enriched: the calls must overlap for the slots to fill", i, e.status)
		}
	}
	if m := most.Load(); m != importEnrichmentConcurrency {
		t.Errorf("at most %d calls were in flight, want exactly %d", m, importEnrichmentConcurrency)
	}
}
