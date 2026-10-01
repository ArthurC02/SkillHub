package ingest

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func modelThatHangsOnTheSecondSkill(t *testing.T, enrichCalls *atomic.Int32) Model {
	t.Helper()
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/enrich-skill", func(w http.ResponseWriter, r *http.Request) {
		if enrichCalls.Add(1) > 1 {
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		writeTestJSON(w, llmclient.EnrichSkillResponse{Summary: testEnrichedSummary, Model: "m", PromptVersion: "p"})
	})
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, llmclient.EmbedResponse{Embeddings: [][]float32{make([]float32, 1536)}, Model: "e", Dimensions: 1536})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return ModelOrNone(&llmclient.Client{BaseURL: srv.URL})
}

func TestAnImportStopsEnrichingWhenItsWindowClosesAndLeavesTheRestForTheBackfill(t *testing.T) {
	var enrichCalls atomic.Int32
	s := &Service{LLM: modelThatHangsOnTheSecondSkill(t, &enrichCalls)}
	admitted := []plannedSkill{{pkg: testPackage()}, {pkg: testPackage()}, {pkg: testPackage()}}

	started := time.Now()
	got := s.enrichWithin(context.Background(), 300*time.Millisecond, admitted, pgtype.UUID{})

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("enrichment took %v; the window should have ended it", elapsed)
	}
	statuses := []enrichmentStatus{got[0].status, got[1].status, got[2].status}
	if statuses[0] != enrichmentEnriched || statuses[1] != enrichmentPending || statuses[2] != enrichmentPending {
		t.Errorf("statuses = %v, want the first enriched and the two the window did not cover left pending", statuses)
	}
	if n := enrichCalls.Load(); n != 2 {
		t.Errorf("the model was asked %d times, want 2: no call may start after the window closed", n)
	}
	if got[2].summary != testDescription {
		t.Errorf("an unreached skill's summary = %q, want its own description", got[2].summary)
	}
}
