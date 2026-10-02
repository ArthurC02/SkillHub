package catalog

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

type countingModel struct {
	embeds, reasons int
	fail, empty     bool
}

func (m *countingModel) Embed(_ context.Context, texts []string, _ time.Duration) (*Embeddings, error) {
	m.embeds++
	if m.fail {
		return nil, errors.New("gateway down")
	}
	vectors := make([][]float32, len(texts))
	if m.empty {
		vectors = nil
	}
	for i := range vectors {
		vectors[i] = []float32{float32(m.embeds)}
	}
	return &Embeddings{Vectors: vectors, Model: "m", Usage: &ModelUsage{PromptTokens: 3}}, nil
}

func (m *countingModel) MatchReasons(_ context.Context, _ string, candidates []SkillCandidate, _ time.Duration) (*MatchReasons, error) {
	m.reasons++
	out := &MatchReasons{Model: "m", Usage: &ModelUsage{PromptTokens: 5}}
	for _, c := range candidates {
		out.Reasons = append(out.Reasons, MatchReason{SkillID: c.SkillID, Reason: "fits"})
	}
	return out, nil
}

type countingAnalyzer struct {
	calls int
	valid bool
}

func (a *countingAnalyzer) AnalyzeIntent(_ context.Context, query string, _ time.Duration) (*IntentAnalysis, error) {
	a.calls++
	return &IntentAnalysis{
		Valid:          a.valid,
		Interpretation: SearchInterpretation{Keywords: []string{query}, Filters: map[string]string{"script": "yes"}, Model: "m", PromptVersion: "v1"},
		Usage:          &ModelUsage{PromptTokens: 7},
	}, nil
}

func TestARepeatedSearchQueryIsAnsweredWithoutAnotherModelCallOrCharge(t *testing.T) {
	inner := &countingModel{}
	m := &cachedModel{Model: inner}
	first, _ := m.Embed(t.Context(), []string{"invoice to table"}, time.Second)
	again, _ := m.Embed(t.Context(), []string{"invoice to table"}, time.Second)
	if inner.embeds != 1 {
		t.Fatalf("the same query was embedded %d times", inner.embeds)
	}
	if first.Usage == nil || again.Usage != nil {
		t.Fatalf("usage first %v again %v; only the call that cost money may carry usage", first.Usage, again.Usage)
	}
	if again.Vectors[0][0] != first.Vectors[0][0] {
		t.Fatal("the cached vector differs from the one the model returned")
	}
	if _, _ = m.Embed(t.Context(), []string{"a different query"}, time.Second); inner.embeds != 2 {
		t.Fatal("a different query was answered from the cache")
	}
}

func TestABatchEmbeddingIsNeverCachedNorServedFromTheCache(t *testing.T) {
	inner := &countingModel{}
	m := &cachedModel{Model: inner}
	texts := []string{"skill a", "skill b"}
	_, _ = m.Embed(t.Context(), texts, time.Second)
	_, _ = m.Embed(t.Context(), texts, time.Second)
	if inner.embeds != 2 {
		t.Fatalf("a batch was served from the cache: %d calls", inner.embeds)
	}
	_, _ = m.Embed(t.Context(), []string{"skill a"}, time.Second)
	if inner.embeds != 3 {
		t.Fatal("a batch's vectors were cached under its first text")
	}
	batch, _ := m.Embed(t.Context(), []string{"skill a", "skill c"}, time.Second)
	if inner.embeds != 4 || len(batch.Vectors) != 2 {
		t.Fatalf("a batch starting with a cached text got %d vectors after %d calls; it was served one cached vector", len(batch.Vectors), inner.embeds)
	}
}

func TestAnAnswerWithoutAVectorIsNotCached(t *testing.T) {
	inner := &countingModel{empty: true}
	m := &cachedModel{Model: inner}
	_, _ = m.Embed(t.Context(), []string{"q"}, time.Second)
	inner.empty = false
	if answer, _ := m.Embed(t.Context(), []string{"q"}, time.Second); len(answer.Vectors) != 1 || inner.embeds != 2 {
		t.Fatalf("after an empty answer the retry got %d vectors from %d calls; the empty answer was cached", len(answer.Vectors), inner.embeds)
	}
}

func TestAFailedModelCallIsNotCached(t *testing.T) {
	inner := &countingModel{fail: true}
	m := &cachedModel{Model: inner}
	_, _ = m.Embed(t.Context(), []string{"q"}, time.Second)
	inner.fail = false
	if answer, err := m.Embed(t.Context(), []string{"q"}, time.Second); err != nil || answer == nil || inner.embeds != 2 {
		t.Fatalf("after a failure the retry got %v, %v with %d calls; the failure was cached", answer, err, inner.embeds)
	}
}

func TestMatchReasonsAreReusedOnlyForTheSameQueryAndCandidates(t *testing.T) {
	inner := &countingModel{}
	m := &cachedModel{Model: inner}
	candidates := []SkillCandidate{{SkillID: "1", Name: "pdf", Summary: "reads pdfs"}}
	_, _ = m.MatchReasons(t.Context(), "pdf", candidates, time.Second)
	hit, _ := m.MatchReasons(t.Context(), "pdf", candidates, time.Second)
	if inner.reasons != 1 || hit.Usage != nil || len(hit.Reasons) != 1 {
		t.Fatalf("calls %d usage %v reasons %v; want one call and a free cached answer", inner.reasons, hit.Usage, hit.Reasons)
	}
	edited := []SkillCandidate{{SkillID: "1", Name: "pdf", Summary: "reads and fills pdfs"}}
	if _, _ = m.MatchReasons(t.Context(), "pdf", edited, time.Second); inner.reasons != 2 {
		t.Fatal("a candidate whose summary changed reused the old reasons")
	}
}

func TestAQueryCannotSpellAnotherQuerysCandidatesIntoItsCacheKey(t *testing.T) {
	inner := &countingModel{}
	m := &cachedModel{Model: inner}
	_, _ = m.MatchReasons(t.Context(), "pdf", []SkillCandidate{{SkillID: "1"}}, time.Second)
	if _, _ = m.MatchReasons(t.Context(), "pdf\x001\x00\x00", nil, time.Second); inner.reasons != 2 {
		t.Fatal("a query ending in the candidate fields' bytes was answered with another search's reasons")
	}
}

func TestOnlyAValidIntentAnalysisIsCachedAndAHitCannotCorruptIt(t *testing.T) {
	invalid := &countingAnalyzer{}
	a := &cachedIntentAnalyzer{IntentAnalyzer: invalid}
	_, _ = a.AnalyzeIntent(t.Context(), "q", time.Second)
	_, _ = a.AnalyzeIntent(t.Context(), "q", time.Second)
	if invalid.calls != 2 {
		t.Fatal("an analysis the model marked invalid was cached")
	}

	valid := &countingAnalyzer{valid: true}
	a = &cachedIntentAnalyzer{IntentAnalyzer: valid}
	first, _ := a.AnalyzeIntent(t.Context(), "q", time.Second)
	first.Interpretation.Filters["validation"] = "verified"
	hit, _ := a.AnalyzeIntent(t.Context(), "q", time.Second)
	hit.Interpretation.Filters["category"] = "docs"
	again, _ := a.AnalyzeIntent(t.Context(), "q", time.Second)
	if valid.calls != 1 || again.Usage != nil {
		t.Fatalf("calls %d usage %v; want one paid call", valid.calls, again.Usage)
	}
	if len(again.Interpretation.Filters) != 1 {
		t.Fatalf("filters %v: a caller's edit reached the cached analysis", again.Interpretation.Filters)
	}
}

func TestACachedAnswerExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	c := &queryCache[int]{now: func() time.Time { return now }}
	c.put("q", 1)
	now = now.Add(queryCacheTTL - time.Second)
	if _, ok := c.get("q"); !ok {
		t.Fatal("an answer was dropped before its time")
	}
	now = now.Add(time.Second)
	if _, ok := c.get("q"); ok {
		t.Fatal("an answer was served past its time")
	}
}

func TestTheCacheHoldsAtMostItsCapacity(t *testing.T) {
	c := &queryCache[int]{}
	for i := range queryCacheEntries + 50 {
		c.put(strconv.Itoa(i), i)
	}
	if n := len(c.entries); n > queryCacheEntries {
		t.Fatalf("the cache grew to %d entries, past its %d", n, queryCacheEntries)
	}
	if _, ok := c.get(strconv.Itoa(queryCacheEntries + 49)); !ok {
		t.Fatal("the newest answer was evicted to make room for itself")
	}
}
